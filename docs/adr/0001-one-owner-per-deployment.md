# One Owner per deployment

Each deployment serves exactly one Owner: it is configured with a single Telegram chat ID and a single Sync Engine, and ignores every other chat. We rejected a hosted multi-tenant service because it would mean holding other people's storage credentials, passing Google's restricted-scope verification for full Drive access, and running accounts and abuse protection, none of which a personal reading tool needs. Anyone who wants it runs their own copy for free.
