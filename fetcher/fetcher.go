package fetcher

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"fmt"
	"io"
	"io/ioutil"
	"mime"
	"mime/quotedprintable"
	"os"
	"strings"
	"time"

	"github.com/emersion/go-imap"
	"github.com/emersion/go-imap/client"
	"github.com/emersion/go-message/mail"
	"github.com/floatpane/matcha/config"
	"go.mozilla.org/pkcs7"
	"golang.org/x/text/encoding/ianaindex"
	"golang.org/x/text/transform"
)

// Attachment holds data for an email attachment.
type Attachment struct {
	Filename         string
	PartID           string // Keep PartID to fetch on demand
	Data             []byte
	Encoding         string // Store encoding for proper decoding
	MIMEType         string // Full MIME type (e.g., image/png)
	ContentID        string // Content-ID for inline assets (e.g., cid: references)
	Inline           bool   // True when the part is meant to be displayed inline
	IsSMIMESignature bool   // True if this attachment is an S/MIME signature
	SMIMEVerified    bool   // True if the S/MIME signature was verified successfully
}

type Email struct {
	UID         uint32
	From        string
	To          []string
	Subject     string
	Body        string
	Date        time.Time
	MessageID   string
	References  []string
	Attachments []Attachment
	AccountID   string // ID of the account this email belongs to
}

func decodePart(reader io.Reader, header mail.PartHeader) (string, error) {
	mediaType, params, err := mime.ParseMediaType(header.Get("Content-Type"))
	if err != nil {
		body, _ := ioutil.ReadAll(reader)
		return string(body), nil
	}

	charset := "utf-8"
	if params["charset"] != "" {
		charset = strings.ToLower(params["charset"])
	}

	encoding, err := ianaindex.IANA.Encoding(charset)
	if err != nil || encoding == nil {
		encoding, _ = ianaindex.IANA.Encoding("utf-8")
	}

	transformReader := transform.NewReader(reader, encoding.NewDecoder())
	decodedBody, err := ioutil.ReadAll(transformReader)
	if err != nil {
		return "", err
	}

	if strings.HasPrefix(mediaType, "multipart/") {
		return "[This is a multipart message]", nil
	}

	return string(decodedBody), nil
}

func decodeHeader(header string) string {
	dec := new(mime.WordDecoder)
	dec.CharsetReader = func(charset string, input io.Reader) (io.Reader, error) {
		encoding, err := ianaindex.IANA.Encoding(charset)
		if err != nil {
			return nil, err
		}
		return transform.NewReader(input, encoding.NewDecoder()), nil
	}
	decoded, err := dec.DecodeHeader(header)
	if err != nil {
		return header
	}
	return decoded
}

func decodeAttachmentData(rawBytes []byte, encoding string) ([]byte, error) {
	switch strings.ToLower(encoding) {
	case "base64":
		decoder := base64.NewDecoder(base64.StdEncoding, bytes.NewReader(rawBytes))
		return ioutil.ReadAll(decoder)
	case "quoted-printable":
		return ioutil.ReadAll(quotedprintable.NewReader(bytes.NewReader(rawBytes)))
	default:
		return rawBytes, nil
	}
}

func connect(account *config.Account) (*client.Client, error) {
	imapServer := account.GetIMAPServer()
	imapPort := account.GetIMAPPort()

	if imapServer == "" {
		return nil, fmt.Errorf("unsupported service_provider: %s", account.ServiceProvider)
	}

	addr := fmt.Sprintf("%s:%d", imapServer, imapPort)

	tlsConfig := &tls.Config{
		ServerName:         imapServer,
		InsecureSkipVerify: account.Insecure,
	}

	var c *client.Client
	var err error

	if imapPort == 1143 || imapPort == 143 {
		c, err = client.Dial(addr)
		if err != nil {
			return nil, err
		}
		if err := c.StartTLS(tlsConfig); err != nil {
			return nil, err
		}
	} else {
		c, err = client.DialTLS(addr, tlsConfig)
		if err != nil {
			return nil, err
		}
	}

	if err := c.Login(account.Email, account.Password); err != nil {
		return nil, err
	}

	return c, nil
}

func getSentMailbox(account *config.Account) string {
	switch account.ServiceProvider {
	case "gmail":
		return "[Gmail]/Sent Mail"
	case "icloud":
		return "Sent Messages"
	default:
		return "Sent"
	}
}

func getMailboxByAttr(c *client.Client, attr string) (string, error) {
	mailboxes := make(chan *imap.MailboxInfo, 10)
	done := make(chan error, 1)
	go func() {
		done <- c.List("", "*", mailboxes)
	}()

	var foundMailbox string
	for m := range mailboxes {
		for _, a := range m.Attributes {
			if a == attr {
				foundMailbox = m.Name
				break
			}
		}
	}

	if err := <-done; err != nil {
		return "", err
	}

	if foundMailbox == "" {
		return "", fmt.Errorf("no mailbox found with attribute %s", attr)
	}

	return foundMailbox, nil
}

func FetchMailboxEmails(account *config.Account, mailbox string, limit, offset uint32) ([]Email, error) {
	c, err := connect(account)
	if err != nil {
		return nil, err
	}
	defer c.Logout()

	mbox, err := c.Select(mailbox, false)
	if err != nil {
		return nil, err
	}

	if mbox.Messages == 0 {
		return []Email{}, nil
	}

	var allEmails []Email
	cursor := uint32(0)
	if mbox.Messages > offset {
		cursor = mbox.Messages - offset
	} else {
		return []Email{}, nil
	}

	fetchEmail := strings.ToLower(strings.TrimSpace(account.FetchEmail))
	if fetchEmail == "" {
		fetchEmail = strings.ToLower(strings.TrimSpace(account.Email))
	}
	isSentMailbox := mailbox == getSentMailbox(account)

	for len(allEmails) < int(limit) && cursor > 0 {
		chunkSize := limit
		if chunkSize < 50 {
			chunkSize = 50
		}

		from := uint32(1)
		if cursor > uint32(chunkSize) {
			from = cursor - uint32(chunkSize) + 1
		}

		seqset := new(imap.SeqSet)
		seqset.AddRange(from, cursor)

		messages := make(chan *imap.Message, chunkSize)
		done := make(chan error, 1)
		fetchItems := []imap.FetchItem{imap.FetchEnvelope, imap.FetchUid}

		go func() {
			done <- c.Fetch(seqset, fetchItems, messages)
		}()

		var batchMsgs []*imap.Message
		for msg := range messages {
			batchMsgs = append(batchMsgs, msg)
		}

		if err := <-done; err != nil {
			return nil, err
		}

		var batchEmails []Email
		for _, msg := range batchMsgs {
			if msg == nil || msg.Envelope == nil {
				continue
			}

			var fromAddr string
			if len(msg.Envelope.From) > 0 {
				fromAddr = msg.Envelope.From[0].Address()
			}

			var toAddrList []string
			for _, addr := range msg.Envelope.To {
				toAddrList = append(toAddrList, addr.Address())
			}
			for _, addr := range msg.Envelope.Cc {
				toAddrList = append(toAddrList, addr.Address())
			}

			matched := false
			if isSentMailbox {
				if strings.EqualFold(strings.TrimSpace(fromAddr), fetchEmail) {
					matched = true
				}
			} else {
				for _, r := range toAddrList {
					if strings.EqualFold(strings.TrimSpace(r), fetchEmail) {
						matched = true
						break
					}
				}
			}

			if !matched {
				continue
			}

			batchEmails = append(batchEmails, Email{
				UID:       msg.Uid,
				From:      fromAddr,
				To:        toAddrList,
				Subject:   decodeHeader(msg.Envelope.Subject),
				Date:      msg.Envelope.Date,
				AccountID: account.ID,
			})
		}

		for i := 0; i < len(batchEmails); i++ {
			for j := i + 1; j < len(batchEmails); j++ {
				if batchEmails[j].UID > batchEmails[i].UID {
					batchEmails[i], batchEmails[j] = batchEmails[j], batchEmails[i]
				}
			}
		}

		allEmails = append(allEmails, batchEmails...)
		cursor = from - 1
	}

	if len(allEmails) > int(limit) {
		allEmails = allEmails[:limit]
	}

	return allEmails, nil
}

func FetchEmailBodyFromMailbox(account *config.Account, mailbox string, uid uint32) (string, []Attachment, error) {
	c, err := connect(account)
	if err != nil {
		return "", nil, err
	}
	defer c.Logout()

	if _, err := c.Select(mailbox, false); err != nil {
		return "", nil, err
	}

	seqset := new(imap.SeqSet)
	seqset.AddNum(uid)

	fetchInlinePart := func(partID, encoding string) ([]byte, error) {
		fetchItem := imap.FetchItem(fmt.Sprintf("BODY.PEEK[%s]", partID))
		section, err := imap.ParseBodySectionName(fetchItem)
		if err != nil {
			return nil, err
		}

		partMessages := make(chan *imap.Message, 1)
		partDone := make(chan error, 1)
		go func() {
			partDone <- c.UidFetch(seqset, []imap.FetchItem{fetchItem}, partMessages)
		}()

		if err := <-partDone; err != nil {
			return nil, err
		}

		partMsg := <-partMessages
		if partMsg == nil {
			return nil, fmt.Errorf("could not fetch inline part %s", partID)
		}

		literal := partMsg.GetBody(section)
		if literal == nil {
			return nil, fmt.Errorf("could not get inline part body %s", partID)
		}

		rawBytes, err := ioutil.ReadAll(literal)
		if err != nil {
			return nil, err
		}

		return decodeAttachmentData(rawBytes, encoding)
	}

	// Replace fetchRawItem with this helper to grab the raw wire-bytes of the entire email
	fetchWholeMessage := func() ([]byte, error) {
		fetchItem := imap.FetchItem("BODY.PEEK[]")
		section, _ := imap.ParseBodySectionName(fetchItem)
		partMessages := make(chan *imap.Message, 1)
		partDone := make(chan error, 1)
		go func() {
			partDone <- c.UidFetch(seqset, []imap.FetchItem{fetchItem}, partMessages)
		}()
		if err := <-partDone; err != nil {
			return nil, err
		}
		partMsg := <-partMessages
		if partMsg != nil {
			literal := partMsg.GetBody(section)
			if literal != nil {
				return ioutil.ReadAll(literal)
			}
		}
		return nil, fmt.Errorf("could not fetch whole message")
	}

	messages := make(chan *imap.Message, 1)
	done := make(chan error, 1)
	fetchItems := []imap.FetchItem{imap.FetchBodyStructure}
	go func() {
		done <- c.UidFetch(seqset, fetchItems, messages)
	}()

	if err := <-done; err != nil {
		return "", nil, err
	}

	msg := <-messages
	if msg == nil || msg.BodyStructure == nil {
		return "", nil, fmt.Errorf("no message or body structure found with UID %d", uid)
	}

	var plainPartID, plainPartEncoding string
	var htmlPartID, htmlPartEncoding string
	var attachments []Attachment
	var checkPart func(part *imap.BodyStructure, partID string)
	checkPart = func(part *imap.BodyStructure, partID string) {
		if part.MIMEType == "text" {
			sub := strings.ToLower(part.MIMESubType)
			switch sub {
			case "html":
				if htmlPartID == "" {
					htmlPartID = partID
					htmlPartEncoding = part.Encoding
				}
			case "plain":
				if plainPartID == "" {
					plainPartID = partID
					plainPartEncoding = part.Encoding
				}
			}
		}

		filename := ""
		if fn, err := part.Filename(); err == nil && fn != "" {
			filename = fn
		}
		if filename == "" {
			if fn, ok := part.DispositionParams["filename"]; ok && fn != "" {
				filename = fn
			}
		}
		if filename == "" {
			if fn, ok := part.Params["name"]; ok && fn != "" {
				filename = fn
			}
		}
		if filename == "" {
			if fn, ok := part.Params["filename"]; ok && fn != "" {
				filename = fn
			}
		}

		contentID := strings.Trim(part.Id, "<>")
		mimeType := fmt.Sprintf("%s/%s", strings.ToLower(part.MIMEType), strings.ToLower(part.MIMESubType))
		isCID := contentID != ""
		isInline := part.Disposition == "inline" || isCID

		if filename == "" && isInline && strings.HasPrefix(mimeType, "image/") {
			filename = "inline"
		}
		if (filename != "" || isCID) && (part.Disposition == "attachment" || isInline || part.MIMEType != "text") {
			att := Attachment{
				Filename:  filename,
				PartID:    partID,
				Encoding:  part.Encoding,
				MIMEType:  mimeType,
				ContentID: contentID,
				Inline:    isInline,
			}
			if att.Inline && strings.HasPrefix(att.MIMEType, "image/") {
				if data, err := fetchInlinePart(partID, part.Encoding); err == nil {
					att.Data = data
				}
			}

			// Validate and Verify S/MIME signatures under the hood
			if filename == "smime.p7s" || mimeType == "application/pkcs7-signature" {
				att.IsSMIMESignature = true
				if data, err := fetchInlinePart(partID, part.Encoding); err == nil {
					att.Data = data
					p7, err := pkcs7.Parse(data)
					if err == nil {
						// Extract the outer multipart/signed boundary
						boundary := msg.BodyStructure.Params["boundary"]
						if boundary != "" {
							// Fetch the RAW, unmodified email bytes
							rawEmail, err := fetchWholeMessage()
							if err == nil {
								fullBoundary := []byte("--" + boundary)

								// Find where the signed content begins
								firstIdx := bytes.Index(rawEmail, fullBoundary)
								if firstIdx != -1 {
									startIdx := firstIdx + len(fullBoundary)
									if startIdx < len(rawEmail) && rawEmail[startIdx] == '\r' {
										startIdx++
									}
									if startIdx < len(rawEmail) && rawEmail[startIdx] == '\n' {
										startIdx++
									}

									// Find where the signed content ends
									secondIdx := bytes.Index(rawEmail[startIdx:], fullBoundary)
									if secondIdx != -1 {
										endIdx := startIdx + secondIdx

										// Strip the trailing CRLF that belongs to the boundary, not the content
										if endIdx > 0 && rawEmail[endIdx-1] == '\n' {
											endIdx--
										}
										if endIdx > 0 && rawEmail[endIdx-1] == '\r' {
											endIdx--
										}

										signedData := rawEmail[startIdx:endIdx]

										// Enforce canonical CRLF line endings as required by S/MIME RFCs
										canonical := bytes.ReplaceAll(signedData, []byte("\r\n"), []byte("\n"))
										canonical = bytes.ReplaceAll(canonical, []byte("\n"), []byte("\r\n"))

										roots, _ := x509.SystemCertPool()
										if roots == nil {
											roots = x509.NewCertPool()
										}

										// Try exact canonical match first
										p7.Content = canonical
										if err := p7.VerifyWithChain(roots); err == nil {
											att.SMIMEVerified = true
										} else {
											// Try with a trailing CRLF (some servers insist on hashing the terminal empty line)
											p7.Content = append(canonical, '\r', '\n')
											if err := p7.VerifyWithChain(roots); err == nil {
												att.SMIMEVerified = true
											} else {
												// Try without any trailing newlines
												p7.Content = bytes.TrimRight(canonical, "\r\n")
												if err := p7.VerifyWithChain(roots); err == nil {
													att.SMIMEVerified = true
												} else {
													if os.Getenv("DEBUG_SMIME") != "" {
														f, _ := os.OpenFile("smime_debug.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
														defer f.Close()
														f.WriteString(fmt.Sprintf("S/MIME Raw Verify Error: %v\n", err))
													}
												}
											}
										}
									}
								}
							}
						}
					}
				}
			}

			attachments = append(attachments, att)
		}
	}

	var findParts func(*imap.BodyStructure, string)
	findParts = func(bs *imap.BodyStructure, prefix string) {
		if len(bs.Parts) == 0 {
			partID := prefix
			if partID == "" {
				partID = "1"
			}
			checkPart(bs, partID)
			return
		}

		for i, part := range bs.Parts {
			partID := fmt.Sprintf("%d", i+1)
			if prefix != "" {
				partID = fmt.Sprintf("%s.%d", prefix, i+1)
			}
			checkPart(part, partID)
			if len(part.Parts) > 0 {
				findParts(part, partID)
			}
		}
	}
	findParts(msg.BodyStructure, "")

	var body string
	textPartID := ""
	textPartEncoding := ""
	if htmlPartID != "" {
		textPartID = htmlPartID
		textPartEncoding = htmlPartEncoding
	} else if plainPartID != "" {
		textPartID = plainPartID
		textPartEncoding = plainPartEncoding
	}
	if os.Getenv("DEBUG_KITTY_IMAGES") != "" {
		msg := fmt.Sprintf("[kitty-img] body selection html=%s plain=%s chosen=%s\n", htmlPartID, plainPartID, textPartID)
		fmt.Print(msg)
		if path := os.Getenv("DEBUG_KITTY_LOG"); path != "" {
			if f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644); err == nil {
				_, _ = f.WriteString(msg)
				_ = f.Close()
			}
		}
	}
	if textPartID != "" {
		partMessages := make(chan *imap.Message, 1)
		partDone := make(chan error, 1)

		fetchItem := imap.FetchItem(fmt.Sprintf("BODY.PEEK[%s]", textPartID))
		section, err := imap.ParseBodySectionName(fetchItem)
		if err != nil {
			return "", nil, err
		}

		go func() {
			partDone <- c.UidFetch(seqset, []imap.FetchItem{fetchItem}, partMessages)
		}()

		if err := <-partDone; err != nil {
			return "", nil, err
		}

		partMsg := <-partMessages
		if partMsg != nil {
			literal := partMsg.GetBody(section)
			if literal != nil {
				buf, _ := ioutil.ReadAll(literal)
				if decoded, err := decodeAttachmentData(buf, textPartEncoding); err == nil {
					body = string(decoded)
				} else {
					body = string(buf)
				}
			}
		}
	}

	return body, attachments, nil
}

func FetchAttachmentFromMailbox(account *config.Account, mailbox string, uid uint32, partID string, encoding string) ([]byte, error) {
	c, err := connect(account)
	if err != nil {
		return nil, err
	}
	defer c.Logout()

	if _, err := c.Select(mailbox, false); err != nil {
		return nil, err
	}

	seqset := new(imap.SeqSet)
	seqset.AddNum(uid)

	fetchItem := imap.FetchItem(fmt.Sprintf("BODY.PEEK[%s]", partID))
	section, err := imap.ParseBodySectionName(fetchItem)
	if err != nil {
		return nil, err
	}

	messages := make(chan *imap.Message, 1)
	done := make(chan error, 1)
	go func() {
		done <- c.UidFetch(seqset, []imap.FetchItem{fetchItem}, messages)
	}()

	if err := <-done; err != nil {
		return nil, err
	}

	msg := <-messages
	if msg == nil {
		return nil, fmt.Errorf("could not fetch attachment")
	}

	literal := msg.GetBody(section)
	if literal == nil {
		return nil, fmt.Errorf("could not get attachment body")
	}

	rawBytes, err := ioutil.ReadAll(literal)
	if err != nil {
		return nil, err
	}

	decoded, err := decodeAttachmentData(rawBytes, encoding)
	if err != nil {
		return rawBytes, nil
	}
	return decoded, nil
}

func moveEmail(account *config.Account, uid uint32, sourceMailbox, destMailbox string) error {
	c, err := connect(account)
	if err != nil {
		return err
	}
	defer c.Logout()

	if _, err := c.Select(sourceMailbox, false); err != nil {
		return err
	}

	seqSet := new(imap.SeqSet)
	seqSet.AddNum(uid)

	return c.UidMove(seqSet, destMailbox)
}

func DeleteEmailFromMailbox(account *config.Account, mailbox string, uid uint32) error {
	c, err := connect(account)
	if err != nil {
		return err
	}
	defer c.Logout()

	if _, err := c.Select(mailbox, false); err != nil {
		return err
	}

	seqSet := new(imap.SeqSet)
	seqSet.AddNum(uid)

	item := imap.FormatFlagsOp(imap.AddFlags, true)
	flags := []interface{}{imap.DeletedFlag}

	if err := c.UidStore(seqSet, item, flags, nil); err != nil {
		return err
	}

	return c.Expunge(nil)
}

func ArchiveEmailFromMailbox(account *config.Account, mailbox string, uid uint32) error {
	c, err := connect(account)
	if err != nil {
		return err
	}
	defer c.Logout()

	var archiveMailbox string
	switch account.ServiceProvider {
	case "gmail":
		archiveMailbox, err = getMailboxByAttr(c, imap.AllAttr)
		if err != nil {
			archiveMailbox = "[Gmail]/All Mail"
		}
	default:
		archiveMailbox = "Archive"
	}

	if _, err := c.Select(mailbox, false); err != nil {
		return err
	}

	seqSet := new(imap.SeqSet)
	seqSet.AddNum(uid)

	return c.UidMove(seqSet, archiveMailbox)
}

func FetchEmails(account *config.Account, limit, offset uint32) ([]Email, error) {
	return FetchMailboxEmails(account, "INBOX", limit, offset)
}

func FetchSentEmails(account *config.Account, limit, offset uint32) ([]Email, error) {
	return FetchMailboxEmails(account, getSentMailbox(account), limit, offset)
}

func FetchEmailBody(account *config.Account, uid uint32) (string, []Attachment, error) {
	return FetchEmailBodyFromMailbox(account, "INBOX", uid)
}

func FetchSentEmailBody(account *config.Account, uid uint32) (string, []Attachment, error) {
	return FetchEmailBodyFromMailbox(account, getSentMailbox(account), uid)
}

func FetchAttachment(account *config.Account, uid uint32, partID string, encoding string) ([]byte, error) {
	return FetchAttachmentFromMailbox(account, "INBOX", uid, partID, encoding)
}

func FetchSentAttachment(account *config.Account, uid uint32, partID string, encoding string) ([]byte, error) {
	return FetchAttachmentFromMailbox(account, getSentMailbox(account), uid, partID, encoding)
}

func DeleteEmail(account *config.Account, uid uint32) error {
	return DeleteEmailFromMailbox(account, "INBOX", uid)
}

func DeleteSentEmail(account *config.Account, uid uint32) error {
	return DeleteEmailFromMailbox(account, getSentMailbox(account), uid)
}

func ArchiveEmail(account *config.Account, uid uint32) error {
	return ArchiveEmailFromMailbox(account, "INBOX", uid)
}

func ArchiveSentEmail(account *config.Account, uid uint32) error {
	return ArchiveEmailFromMailbox(account, getSentMailbox(account), uid)
}

func getTrashMailbox(account *config.Account) string {
	switch account.ServiceProvider {
	case "gmail":
		return "[Gmail]/Trash"
	case "icloud":
		return "Deleted Messages"
	default:
		return "Trash"
	}
}

func getArchiveMailbox(account *config.Account) string {
	switch account.ServiceProvider {
	case "gmail":
		return "[Gmail]/All Mail"
	case "icloud":
		return "Archive"
	default:
		return "Archive"
	}
}

func FetchTrashEmails(account *config.Account, limit, offset uint32) ([]Email, error) {
	c, err := connect(account)
	if err != nil {
		return nil, err
	}
	defer c.Logout()

	trashMailbox, err := getMailboxByAttr(c, imap.TrashAttr)
	if err != nil {
		trashMailbox = getTrashMailbox(account)
	}

	return FetchMailboxEmails(account, trashMailbox, limit, offset)
}

func FetchArchiveEmails(account *config.Account, limit, offset uint32) ([]Email, error) {
	c, err := connect(account)
	if err != nil {
		return nil, err
	}
	defer c.Logout()

	archiveMailbox, err := getMailboxByAttr(c, imap.AllAttr)
	if err != nil {
		archiveMailbox = getArchiveMailbox(account)
	}

	mbox, err := c.Select(archiveMailbox, false)
	if err != nil {
		return nil, err
	}

	if mbox.Messages == 0 {
		return []Email{}, nil
	}

	to := mbox.Messages - offset
	from := uint32(1)
	if to > limit {
		from = to - limit + 1
	}

	if to < 1 {
		return []Email{}, nil
	}

	seqset := new(imap.SeqSet)
	seqset.AddRange(from, to)

	messages := make(chan *imap.Message, limit)
	done := make(chan error, 1)
	fetchItems := []imap.FetchItem{imap.FetchEnvelope, imap.FetchUid}
	go func() {
		done <- c.Fetch(seqset, fetchItems, messages)
	}()

	var msgs []*imap.Message
	for msg := range messages {
		msgs = append(msgs, msg)
	}

	if err := <-done; err != nil {
		return nil, err
	}

	fetchEmail := strings.ToLower(strings.TrimSpace(account.FetchEmail))
	if fetchEmail == "" {
		fetchEmail = strings.ToLower(strings.TrimSpace(account.Email))
	}

	var emails []Email
	for _, msg := range msgs {
		if msg == nil || msg.Envelope == nil {
			continue
		}

		var fromAddr string
		if len(msg.Envelope.From) > 0 {
			fromAddr = msg.Envelope.From[0].Address()
		}

		var toAddrList []string
		for _, addr := range msg.Envelope.To {
			toAddrList = append(toAddrList, addr.Address())
		}
		for _, addr := range msg.Envelope.Cc {
			toAddrList = append(toAddrList, addr.Address())
		}

		matched := false
		if strings.EqualFold(strings.TrimSpace(fromAddr), fetchEmail) {
			matched = true
		}
		if !matched {
			for _, r := range toAddrList {
				if strings.EqualFold(strings.TrimSpace(r), fetchEmail) {
					matched = true
					break
				}
			}
		}

		if !matched {
			continue
		}

		emails = append(emails, Email{
			UID:       msg.Uid,
			From:      fromAddr,
			To:        toAddrList,
			Subject:   decodeHeader(msg.Envelope.Subject),
			Date:      msg.Envelope.Date,
			AccountID: account.ID,
		})
	}

	for i, j := 0, len(emails)-1; i < j; i, j = i+1, j-1 {
		emails[i], emails[j] = emails[j], emails[i]
	}

	return emails, nil
}

func FetchTrashEmailBody(account *config.Account, uid uint32) (string, []Attachment, error) {
	c, err := connect(account)
	if err != nil {
		return "", nil, err
	}
	defer c.Logout()

	trashMailbox, err := getMailboxByAttr(c, imap.TrashAttr)
	if err != nil {
		trashMailbox = getTrashMailbox(account)
	}

	return FetchEmailBodyFromMailbox(account, trashMailbox, uid)
}

func FetchArchiveEmailBody(account *config.Account, uid uint32) (string, []Attachment, error) {
	c, err := connect(account)
	if err != nil {
		return "", nil, err
	}
	defer c.Logout()

	archiveMailbox, err := getMailboxByAttr(c, imap.AllAttr)
	if err != nil {
		archiveMailbox = getArchiveMailbox(account)
	}

	return FetchEmailBodyFromMailbox(account, archiveMailbox, uid)
}

func FetchTrashAttachment(account *config.Account, uid uint32, partID string, encoding string) ([]byte, error) {
	c, err := connect(account)
	if err != nil {
		return nil, err
	}
	defer c.Logout()

	trashMailbox, err := getMailboxByAttr(c, imap.TrashAttr)
	if err != nil {
		trashMailbox = getTrashMailbox(account)
	}

	return FetchAttachmentFromMailbox(account, trashMailbox, uid, partID, encoding)
}

func FetchArchiveAttachment(account *config.Account, uid uint32, partID string, encoding string) ([]byte, error) {
	c, err := connect(account)
	if err != nil {
		return nil, err
	}
	defer c.Logout()

	archiveMailbox, err := getMailboxByAttr(c, imap.AllAttr)
	if err != nil {
		archiveMailbox = getArchiveMailbox(account)
	}

	return FetchAttachmentFromMailbox(account, archiveMailbox, uid, partID, encoding)
}

func DeleteTrashEmail(account *config.Account, uid uint32) error {
	c, err := connect(account)
	if err != nil {
		return err
	}
	defer c.Logout()

	trashMailbox, err := getMailboxByAttr(c, imap.TrashAttr)
	if err != nil {
		trashMailbox = getTrashMailbox(account)
	}

	return DeleteEmailFromMailbox(account, trashMailbox, uid)
}

func DeleteArchiveEmail(account *config.Account, uid uint32) error {
	c, err := connect(account)
	if err != nil {
		return err
	}
	defer c.Logout()

	archiveMailbox, err := getMailboxByAttr(c, imap.AllAttr)
	if err != nil {
		archiveMailbox = getArchiveMailbox(account)
	}

	return DeleteEmailFromMailbox(account, archiveMailbox, uid)
}
