package main

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"
	pb "github.com/schollz/progressbar/v3"
	"github.com/sirupsen/logrus"
	ffprobe "github.com/vansante/go-ffprobe"
)

const (
	MimeTypeOpus         = "audio/ogg; codecs=opus"
	SyntaxErrorSemicolon = "syntax error, \";\" should be the last character in command string and only be used once"

	schema = `
	drop table if exists chat;

	create table chat (
		id integer not null primary key,
		from_me tinyint(1) not null,
		latitude real,
		longitude real,
		key_id varchar(255),
		media_filepath varchar(255),
		media_job_uuid varchar(255),
		media_mime_type varchar(255),
		media_transcription text,
		media_audio_length_seconds real,
		media_file_size_byte sqlite3_int64,
		chat_id integer,
		reply_to varchar(255),
		sender_contact_name varchar(255),
		sender_contact_number varchar(255),
		sender_contact_raw_string_jid varchar(255),
		text_data text,
		timestamp integer
	);
	`

	insert = `
		insert into chat values (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`
)

var (
	commandBuffer []string
)

type GeoPosition struct {
	Latitude  *float32 `json:"latitude"`
	Longitude *float32 `json:"longitude"`
	MessageId *int64   `json:"message_id"`
}

type Media struct {
	FilePath           *string  `json:"file_path"`
	MediaJobUuid       *string  `json:"media_job_uuid"`
	MessageId          *int64   `json:"message_id"`
	MimeType           *string  `json:"mime_type"`
	Transcription      *string  `json:"transcription"`
	AudioLengthSeconds *float64 `json:"audio_length_seconds"`
	FileSizeByte       *int64   `json:"file_size_byte"`
}

type SenderContact struct {
	Name         *string `json:"name"`
	Number       *string `json:"number"`
	RawStringJid *string `json:"raw_string_jid"`
}

type Message struct {
	ChatId        *int           `json:"chat_id"`
	FromMe        *int           `json:"from_me"`
	GeoPosition   *GeoPosition   `json:"geo_position"`
	KeyId         *string        `json:"key_id"`
	Media         *Media         `json:"media"`
	MessageId     *int64         `json:"message_id"`
	ReplyTo       *string        `json:"reply_to"`
	SenderContact *SenderContact `json:"sender_contact"`
	TextData      *string        `json:"text_data"`
	Timestamp     *int64         `json:"timestamp"`
}

type ChatTitle struct {
	Name         *string `json:"name"`
	Number       *string `json:"number"`
	RawStringJid *string `json:"raw_string_jid"`
}

type Chat struct {
	ChatId    *int       `json:"chat_id"`
	ChatTitle *ChatTitle `json:"chat_title"`
	Messages  []*Message `json:"messages"`
}

func main() {
	chatFile := flag.String("chat_file", "", "file of the chat to analyze")
	workDir := flag.String("workdir", ".", "directory path to the WhatsApp folder containing the Media and Database folders")
	transcriptAudioFiles := flag.Bool("transcript_audio", false, "wether the voice messages should be transcripted or not")
	transcriptionQuality := flag.String("transcription_quality", "medium", "low, medium, high (high takes a lot of time)")
	forceTranscription := flag.Bool("force_transcription", false, "if set to true existing transcriptions are replaced")
	language := flag.String("language", "", "the language of the majority of chats for transcription, if nothing is set, autodetect is used")
	dbName := flag.String("db", "chat.db", "path to the output database")
	flag.Parse()

	content, err := os.ReadFile(*chatFile)
	if err != nil {
		logrus.Fatalln(err)
	}

	chat := &Chat{}
	err = json.Unmarshal(content, chat)
	if err != nil {
		logrus.WithError(err).Fatalln("unable to read chat file")
	}

	fileCount, opusFileCount := getMediaAndMessageFileCount(chat)

	getMetadataInformation(chat, *workDir, fileCount)

	if *transcriptAudioFiles {
		generateAudioTranscripts(chat, *transcriptionQuality, *language, *workDir, opusFileCount, *forceTranscription)
	}

	storeToDb(chat, *dbName)
}

func storeToDb(chat *Chat, file string) {
	bar := createProgressBar(len(chat.Messages), "inserting messages into database")

	db, err := sql.Open("sqlite3", file)
	if err != nil {
		logrus.WithError(err).Fatalln("error while opening database file")
	}
	defer db.Close()

	if _, err := db.Exec(schema); err != nil {
		logrus.WithError(err).Fatalln("error while creating database schema")
	}

	for _, message := range chat.Messages {
		var latitude *float32
		var longitude *float32

		if message.GeoPosition != nil && message.GeoPosition.Latitude != nil && message.GeoPosition.Longitude != nil {
			latitude = message.GeoPosition.Latitude
			longitude = message.GeoPosition.Longitude
		}

		var mediaAudioLengthSeconds *float64
		var mediaFilePath *string
		var mediaFileSizeByte *int64
		var mediaJobUuid *string
		var mediaMimeType *string
		var mediaTranscription *string

		if message.Media != nil {
			mediaAudioLengthSeconds = message.Media.AudioLengthSeconds
			mediaFilePath = message.Media.FilePath
			mediaFileSizeByte = message.Media.FileSizeByte
			mediaJobUuid = message.Media.MediaJobUuid
			mediaMimeType = message.Media.MimeType

			if message.Media.Transcription != nil {
				mediaTranscription = message.Media.Transcription
			}
		}

		var senderContactName *string
		var senderContactNumber *string
		var senderContactRawJidString *string

		if message.SenderContact != nil {
			senderContactName = message.SenderContact.Name
			senderContactNumber = message.SenderContact.Number
			senderContactRawJidString = message.SenderContact.RawStringJid
		}

		_, err := db.Exec(insert,
			message.MessageId,
			message.FromMe,
			latitude,
			longitude,
			message.KeyId,
			mediaFilePath,
			mediaJobUuid,
			mediaMimeType,
			mediaTranscription,
			mediaAudioLengthSeconds,
			mediaFileSizeByte,
			message.ChatId,
			message.ReplyTo,
			senderContactName,
			senderContactNumber,
			senderContactRawJidString,
			message.TextData,
			message.Timestamp)
		if err != nil {
			logrus.WithError(err).Errorln("error while inserting message to db")
		}
		bar.Add(1)
	}

}

// return: media messages, opus media messages
func getMediaAndMessageFileCount(chat *Chat) (int, int) {
	cnt := 0
	opusCnt := 0
	for _, message := range chat.Messages {
		if message != nil && message.Media != nil && message.Media.FilePath != nil {
			cnt++
			if message.Media.MimeType != nil && *message.Media.MimeType == MimeTypeOpus {
				opusCnt++
			}
		}
	}
	return cnt, opusCnt
}

func createProgressBar(count int, description string) *pb.ProgressBar {
	return pb.NewOptions(count,
		pb.OptionFullWidth(),
		pb.OptionThrottle(0),
		pb.OptionEnableColorCodes(true),
		pb.OptionShowIts(),
		pb.OptionShowCount(),
		pb.OptionSetDescription(description),
		pb.OptionOnCompletion(func() {
			fmt.Println()
			fmt.Println()
		}),
	)
}

func getMetadataInformation(chat *Chat, workdir string, fileCount int) {
	bar := createProgressBar(fileCount, "retreiving media file metadata")

	for _, message := range chat.Messages {
		if message == nil || message.Media == nil || message.Media.FilePath == nil {
			continue
		}
		bar.Add(1)

		absPath := workdir + "/" + *message.Media.FilePath

		info, err := os.Stat(absPath)
		if err != nil {
			continue
		}

		var fileSize int64
		fileSize = info.Size()
		message.Media.FileSizeByte = &fileSize

		if message.Media.MimeType != nil && *message.Media.MimeType != MimeTypeOpus {
			continue
		}

		transcription, err := os.ReadFile(absPath + ".txt")
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			logrus.WithError(err).Warnf("error reading transcription of audio file %s", absPath)
		} else {
			transcriptionPtr := strings.TrimSpace(string(transcription))
			message.Media.Transcription = &transcriptionPtr
		}

		data, err := ffprobe.GetProbeData(absPath, 5*time.Second)
		if err != nil {
			continue
		}

		var length float64
		length = data.Format.DurationSeconds
		message.Media.AudioLengthSeconds = &length
	}
}

func generateAudioTranscripts(chat *Chat, quality string, language string, workdir string, opusFileCount int, force bool) {
	bar := createProgressBar(opusFileCount, "transcripting audio files")
	bar.Set(0)

	var sz string
	switch quality {
	case "low":
		sz = "small"
	case "medium":
		sz = "medium"
	case "high":
		sz = "large"
	default:
		sz = "medium"
	}

	for _, message := range chat.Messages {
		if message == nil || message.Media == nil || message.Media.FilePath == nil || message.Media.MimeType == nil || *message.Media.MimeType != MimeTypeOpus {
			continue
		}
		media := message.Media

		absPath := workdir + "/" + *media.FilePath
		if _, err := os.Stat(absPath); errors.Is(err, os.ErrNotExist) {
			logrus.Errorf("audio file %s doesn't seem to exist anymore", absPath)
			continue
		}

		errNotExist := false
		if _, err := os.Stat(absPath + ".txt"); errors.Is(err, os.ErrNotExist) {
			errNotExist = true
		} else if err != nil && !errors.Is(err, os.ErrExist) {
			logrus.WithError(err).Errorf("error while transcripting file %s", absPath)
			continue
		}

		if errNotExist || force {
			logrus.Infof("transcripting file %s", absPath)

			args := []string{
				absPath,
				"--model",
				sz,
				"--output_dir",
				absPath[:strings.LastIndex(absPath, "/")],
			}

			if language != "" {
				args = append(args, "--language")
				args = append(args, language)
			}

			cmd := exec.Command("whisper", args...)

			var out bytes.Buffer
			var stderr bytes.Buffer
			cmd.Stdout = &out
			cmd.Stderr = &stderr
			err := cmd.Run()
			logrus.Infoln(out.String())
			if err != nil {
				logrus.WithError(err).Errorln("error occured during transcription")
				continue
			}
		}

		transcription, err := os.ReadFile(absPath + ".txt")
		if err != nil {
			logrus.WithError(err).Errorln("error reading transcription file")
			continue
		}
		var transStr string
		transStr = string(transcription)

		message.Media.Transcription = &transStr

		bar.Add(1)
	}
}
