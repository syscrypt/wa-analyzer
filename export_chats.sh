#!/usr/bin/env bash
DIR="workdir_john_doe"
WORKDIR="generated/"$DIR

KEY_EXTRACT_TOOL="WhatsApp-Key-Database-Extractor"
CHAT_EXPORT_TOOL="whatsapp-backup-chat-viewer"

echo "creating workdir "$WORKDIR
mkdir -p $WORKDIR

if [[ $(find ./ -name $KEY_EXTRACT_TOOL | wc -l) == 0 ]]; then
    echo "cloning whatsapp key extraction tool..."
    git clone https://github.com/YuvrajRaghuvanshiS/WhatsApp-Key-Database-Extractor.git
fi

if [[ $(find ./ -name $CHAT_EXPORT_TOOL | wc -l) == 0 ]]; then
    echo "cloning whatsapp chat export tool..."
    git clone https://github.com/Dexter2389/whatsapp-backup-chat-viewer.git
fi

if [[ $(ls /run/user/$UID/gvfs | wc -l) > 0 ]]; then
    echo "mtp device is already mounted"
else
    echo "mounting mtp device..."

    #mount android phone
    gio mount -li | awk -F= '{if(index($2,"mtp") == 1)system("gio mount "$2)}'
    echo "mounted mtp device successfully"
fi

echo "locating whatsapp directory on device..."

MTPDEVICE="$(ls /run/user/$UID/gvfs)"
STORAGE="$(ls /run/user/$UID/gvfs/"$MTPDEVICE")"
MSGSTORE="$(find /run/user/$UID/gvfs/"$MTPDEVICE"/"$STORAGE"/Android/media/ -iname "msgstore.db*" -printf '%h\n' -quit)"

if [[ $(printf $MSGSTORE | wc -c) < 20 ]]; then
    echo "couldn't find directory on expected location, starting extended search..."
    MSGSTORE="$(find /run/user/$UID/gvfs/"$MTPDEVICE"/"$STORAGE"/ -iname "msgstore.db*" -printf '%h\n' -quit)"

    if [[ $(printf $MSGSTORE | wc -c) < 20 ]]; then
        "couldn't find whatsapp directory on device...shutting down"
        exit
    fi
fi

CUR_DIR="$(pwd)"
cd "$MSGSTORE"/../../ && cp -rv ./WhatsApp "$CUR_DIR"/"$WORKDIR" && cd "$CUR_DIR"

echo "running key extraction tool"
cd "$KEY_EXTRACT_TOOL" && python3 wa_kdbe.py && cd "$CUR_DIR"

echo "exporting messages..."
cd "$CHAT_EXPORT_TOOL" && python3 main.py -mdb "$CUR_DIR"/"$WORKDIR"/msgstore.db -wdb "$CUR_DIR"/"$WORKDIR"/wa.db -o "$CUR_DIR"/"$WORKDIR"/exported_chats -f json && cd "$CUR_DIR"

#unmount android phone
gio mount -li | awk -F= '{if(index($2,"mtp") == 1)system("gio mount -u "$2)}'
