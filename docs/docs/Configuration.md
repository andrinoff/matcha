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
  ]
}
```

## Additional Data Locations

- **Drafts**: `~/.config/matcha/drafts/`
- **Email Cache**: `~/.config/matcha/cache.json`
- **Contacts**: `~/.config/matcha/contacts.json`
