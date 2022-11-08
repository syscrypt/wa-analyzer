# Whatsapp Chat Analyzer

Small tool for backing up and exporting your Whatsapp chats for analyzing purposes. The tool performs metadata enrichment by analyzing sent and received mime types as well as a full transcription of voice messages via OpenAIs whisper tool. The output is a searchable sqlite database.

## Dependencies

In order to run this tool you need:

- An android device with Whatsapp
- git cli
- gvfs mount and gio
- [whisper](https://openai.com/blog/whisper/)

## Build

To build this project you need the go compiler toolchain. Simply execute `go build` within the root folder of the project.

## Usage of the export script

Clone this repository via git cli or download the zip.

```shell
git clone https://github.com/syscrypt/wa-analyzer
```

First change the `DIR` variable within the `export_chats.sh` script to a name that fits your needs.

Activate the USB-Debugging option in your android device and connect the unlocked phone to your computer. Execute the `export_chats.sh` script. This script will clone a project into your working directory which is able to unlock the Whatsapp databases. For details and support visit:
https://github.com/YuvrajRaghuvanshiS/WhatsApp-Key-Database-Extractor

After that it will also download another project which extracts the message databases and transforms them into json format. For details and support visit:
https://github.com/Dexter2389/whatsapp-backup-chat-viewer

After cloning both projects the script makes a full backup of the Whatsapp folder on your phone and stores it into the working directory defined by the `DIR` variable. Then it executes the Database Key Extraction tool. Omit the update promoted by the legacy Whatsapp version and click on the proceed save button on your phone. When your decrypted files show up save them in the root of the working directory you defined in the `DIR` variable.

The script proceeds to export the messages into json format and unmounts the device after that.

ToDos:

- build docker container
- option to perform the key extraction on android emulator instead of the real device

## Usage of wa-analyzer

After building the executable you can call `wa-analyzer` with the following arguments:
| param | type | default | description |
|-------------------------|----------|-----------|-------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `chat_file` | `string` | "" | path to the chat file to analyze |
| `workdir` | `string` | "." | directory path to the WhatsApp folder containing the Media and Database folders |
| `transcript_audio` | `bool` | false | perform a transcription of audio files |
| `transcription_quality` | `string` | "medium" | low, medium, high (high takes a lot of time) |
| `force_transcription` | `bool` | false | if set to true, existing transcriptions are replaced |
| `language` | `string` | "" | the language of the majority of chats for transcription, if nothing is set, autodect is used. For all options please refer to OpenAIs whisper documentation |
| `db_name` | `string` | "chat.db" | path to the output database |
