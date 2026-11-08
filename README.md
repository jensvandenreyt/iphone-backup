# iphone-backup

A terminal app for browsing WhatsApp chats in an iPhone backup and exporting their media to disk.

It runs on macOS and Windows and reads the backups that Apple Configurator, Finder, iTunes and the Apple Devices app
create, encrypted or not. Nothing is uploaded or changed in the
backup. Decrypted data is kept in a private temporary folder that is removed when the app exits, and only the media you
export is written elsewhere.

This app is created for people like me, with an iPhone model with, to current standards, limited diskspace. In my case,
creating a new WhatsApp backup or iCloud backup is not possible, simply because there is not enough space left on the
device. When looking for methods to be able to free up some diskspace (my WhatsApp media library was 17GB) without
losing my media, you easily come across software that will very soon hit you with a paywall.

Since I don't like that, and I have access to Claude, I thought I let it vibe-code a small app to do exactly what I
need.
If you are in a similar situation, feel free to use this app to create an export of all your WhatsApp media.

This app was tested using an iPhone 11 and Apple Configurator 2.21. I don't expect many differences for other models,
but that has not been tested.

## Functionality

### Opening a backup

On start, the app lists the backups in the default backup folder with the device name, iOS version, backup date and
whether the backup is encrypted. The default folder is `~/Library/Application Support/MobileSync/Backup` on macOS. On
Windows it is `%USERPROFILE%\Apple\MobileSync\Backup` for the Apple Devices app and iTunes from the Microsoft Store, or
`%APPDATA%\Apple Computer\MobileSync\Backup` for iTunes from apple.com, whichever exists. "Open other directory..." lets you
type the path to a single backup or to another folder of backups. You can also pass one with `-dir`:

```
iphone-backup -dir /path/to/backup
```

On macOS, reading the default backup folder needs Full Disk Access for your terminal, which you can grant in System
Settings > Privacy & Security.

### Unlocking

For an encrypted backup, the app asks for the backup password. Deriving the key takes a few seconds by design. A wrong
password is reported and you can try again. Unencrypted backups open without a prompt.

### Browsing chats

Chats are listed with the most recent first, showing the number of messages and media files and the date of the last
message. Press `/` to filter by name.

Opening a chat shows its messages with date, sender and text, with media shown as a marker such as
`[image: name.jpg, 1.2 MB]`. Chats with hundreds of thousands of messages load in the background and scroll smoothly.

| Key                | Action                                                                        |
|--------------------|-------------------------------------------------------------------------------|
| `enter`            | Open the chat's messages                                                      |
| `m`                | Open the chat's media                                                         |
| `space`            | Select or deselect a chat for exporting                                       |
| `a`                | Select or deselect all visible chats                                          |
| `e`                | Export all media of the selected chats, or of all chats when none is selected |
| `↑` `↓` or `k` `j` | Scroll one message                                                            |
| `pgup` `pgdn`      | Scroll one screen                                                             |
| `home` `end`       | Jump to the first or last message                                             |
| `esc`              | Go back                                                                       |
| `q`                | Quit from the chat list                                                       |
| `ctrl+c`           | Quit from anywhere                                                            |

### Exporting media

The media list shows every image, video, voice note, document, GIF and sticker in a chat, with its date, sender, type
and size. Files that are referenced in the chat but missing from the backup are greyed out.

| Key     | Action                                                              |
|---------|---------------------------------------------------------------------|
| `space` | Select or deselect a file                                           |
| `a`     | Select or deselect all visible files                                |
| `t`     | Filter by type                                                      |
| `/`     | Filter by sender, type or file name                                 |
| `e`     | Export the selection, or all visible files when nothing is selected |

Export asks for a target folder, `export/<chat name>` by default, and shows a progress bar with a window of the last 10
files processed. Files are named after the message, for example `2024-03-01_14-32-05_Alice_photo.jpg`, and get the
message date as their modification time. Documents keep their original title.

Running the same export again skips files that are already in the target folder. A different file with the same name
gets a `-1` suffix. The summary at the end lists how many files were exported, skipped, missing from the backup or
failed.

### Exporting all media at once

To export the media of many chats in one go, select them in the chat list with `space` or `a` and press `e`. Without a
selection, all chats are exported. After a short moment to collect the media, the same target folder dialog appears,
with `export` as the default. Every selected chat gets its own subfolder named after the chat, and all of its media is
exported into it, as if you had selected all files in that chat. Chats that share a name get their chat ID added to the
folder name. Skipping files that were already exported works the same as for a single chat, so an interrupted export
can simply be started again.

### Logging

Every important operation is logged, including which file failed and why. The log is written to
`~/Library/Logs/iphone-backup/iphone-backup.log` on macOS and `%LOCALAPPDATA%\iphone-backup\iphone-backup.log` on
Windows. Use `-log` to write the log somewhere else. Passwords and message text are never logged.

## Building

Requires Go and [Task](https://taskfile.dev).

```
task build           # builds bin/iphone-backup for this computer (.exe on Windows)
task build:windows   # builds bin/iphone-backup.exe for Windows, from any operating system
task build:mac       # builds bin/iphone-backup-mac for Apple silicon Macs, from any operating system
task test            # runs the tests
```

On Windows, run the app in Windows Terminal for the best result.

## Contributing

Contrary to my preference, I didn't write a single line of this code. Instead, Claude did all the work.

If you feel up to the task and want to contribute, be my guest. Just create a PR, I'll have a look.
