# url2supernote

Sends web articles to an Owner's Supernote so they can be read offline.

## Language

**Owner**:
The one person a deployment serves, identified by a single Telegram chat and paired with a single Sync Engine.
_Avoid_: User, customer, tenant, account

**Article**:
A web page, identified by its URL, that the Owner wants to read on their Supernote.
_Avoid_: Link, page, post

**Sync Engine**:
A cloud storage provider the Supernote natively syncs with, such as Google Drive, Dropbox, or OneDrive.
_Avoid_: Storage backend, provider, cloud, drive

**Inbox**:
The `INBOX` folder the Supernote keeps inside its sync folder on a Sync Engine, and the folder Owners normally choose as their Destination.
_Avoid_: Upload folder

**Destination**:
The folder on the Owner's Sync Engine where Deliveries land.
_Avoid_: Target, output folder

**Delivery**:
One attempt to turn an Article into a readable file and place it in the Destination, ending in exactly one outcome: delivered, duplicate, or failed.
_Avoid_: Job, send, upload, conversion
