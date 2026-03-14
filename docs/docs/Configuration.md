# Configuration

Configuration is stored in `~/.config/matcha/config.json`.

## Example Configuration

> Passwords have been removed since [v0.19.0](https://github.com/floatpane/matcha/releases/tag/v0.19.0)

```json
{
  "accounts": [
    {
      "id": "unique-id-1",
      "name": "John Doe",
      "email": "john@gmail.com",
      "service_provider": "gmail",
      "fetch_email": "john@gmail.com"
      "smime_cert": "/home/jane/.certs/jane_smime_cert.pem",
      "smime_key": "/home/jane/.certs/jane_smime_private.pem"
    },
    {
      "id": "unique-id-2",
      "name": "Work Email",
      "email": "john@company.com",
      "service_provider": "custom",
      "fetch_email": "john@company.com",
      "imap_server": "imap.company.com",
      "imap_port": 993,
      "smtp_server": "smtp.company.com",
      "smtp_port": 587
    }
  ],
  "mailing_lists": [
    {
      "name": "Team",
      "addresses": ["alice@example.com", "bob@example.com"]
    }
  ],
  "theme": "Matcha",
  "disable_images": true,
  "hide_tips": true
}
```

## Additional Data Locations

- **Drafts**: `~/.config/matcha/drafts/`
- **Email Cache**: `~/.config/matcha/cache.json`
- **Contacts**: `~/.config/matcha/contacts.json`
- **Custom Themes**: `~/.config/matcha/themes/*.json`
