# Admin Console

A beautiful, secure admin configuration page for Fusion Search.

**✨ Static files are now embedded in the binary** - no separate deployment needed!

## Features

- 🔐 **Password-protected access** - Secure login with session management
- ⚙️ **Full config management** - Edit all configuration settings via a beautiful UI
- 🔑 **Password change** - Update admin password directly from the interface
- 🎨 **Dark theme** - Matches the Fusion Search developer-centric aesthetic
- 📱 **Responsive design** - Works on desktop and mobile devices

## Access

The admin console is available at: `http://your-server:9000/admin`

### Default Credentials

- **Password**: `admin123`

**⚠️ Important**: Change the default password immediately after first login!

## Configuration Sections

### Server
- Host and port settings

### Search
- Backend provider (SearXNG/DuckDuckGo)
- API credentials
- Search engine URLs

### Extraction
- Concurrent request limits
- Timeout settings
- User agents for web scraping

### Cache
- Redis configuration
- TTL settings for search and extracted content

### Auth
- API key authentication
- Enable/disable API access

### LLM
- AI model configuration
- Provider settings (Ollama, OpenAI, Groq, etc.)
- Model selection and parameters

### Advanced
- Proxy settings
- Rate limiting
- Reranking
- Resilience (circuit breaker, retries)
- Logging
- CORS

## Security

- Password hash is stored in `config.yaml` under the `admin.password_hash` field
- Password changes are **persisted** to the config file
- Session tokens expire after 24 hours
- All admin API endpoints require authentication
- Password change requires current password verification

## Technical Details

- **Frontend**: Pure HTML/CSS/JavaScript (no dependencies)
- **Backend**: Go handlers with session management
- **Config format**: YAML (automatically converted from JSON)
- **Storage**: Config written directly to `config.yaml`

## Usage Tips

1. **Tab Navigation**: Use tabs to navigate between configuration sections
2. **Array Fields**: Click "+ Add" buttons to add multiple values (API keys, user agents, etc.)
3. **Save Changes**: Click "Save Configuration" to write changes to disk
4. **Reset**: Click "Reset" to reload the current configuration without saving
5. **Toast Notifications**: Success/error messages appear in the bottom-right corner

## Customization

To customize the default password, modify the `admin.password_hash` field in `config.yaml`:

```yaml
admin:
  password_hash: "your-sha256-hash-here"
```

Generate a SHA256 hash:
```bash
echo -n "your-password" | sha256sum
```

Or simply use the admin panel to change the password after logging in.
