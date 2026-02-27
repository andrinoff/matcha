# Contact Management

Matcha keeps your contacts organized and easily accessible.

## Features

- **📇 Automatic Contact Saving**: Email addresses are automatically saved from emails you receive and send.
- **🔍 Smart Search**: Fuzzy search through your contacts while composing.
- **⚡ Quick Autocomplete**: Contact suggestions appear as you type in the "To" field.
- **💾 Persistent Storage**: Contacts are saved locally for offline access.

## Mailing Lists

You can easily define mailing lists to send emails to the same multiple recipients. These are added directly to your `~/.config/matcha/config.json`.

```json
{
  "mailing_lists": [
    {
      "name": "Team",
      "addresses": ["alice@example.com", "bob@example.com"]
    }
  ]
}
```

Once defined, you can just type the name of your mailing list (e.g., `Team`) in the "To" field and hit `Tab` or `Enter` to auto-complete the list of addresses.
