# Google Drive access through drive.file and the Google Picker

The Owner grants access to the Destination folder by picking it in the Google Picker during `auth google`, and the bot asks only for the `drive.file` scope; it refuses to start with a token that grants more. We previously asked for full Drive access because the Supernote, not url2supernote, creates `INBOX`, so a refresh token that leaked from the bot's environment could read and delete the Owner's whole Drive. Tested against a real Drive, `drive.file` lets the bot read the picked folder, add files to it, and find, change and delete the files it created; reading, downloading or editing anything else is refused.

## Consequences

- The Destination is configured by folder ID rather than path, because `drive.file` cannot look folders up by name. If the Supernote recreates `INBOX`, the Owner picks it again.
- The duplicate check only sees files the bot created, so a file the Owner added by hand with the same name does not count as a duplicate.
- `drive.file` access belongs to the Google Cloud project rather than the OAuth client: a new client in the same project can see files an earlier client created, while a different project starts with an empty duplicate history.
- A leaked token can still create new files in any folder whose ID it knows, such as the Supernote sync folder, which Drive returns as `INBOX`'s parent. It cannot read them back or touch existing files there.
