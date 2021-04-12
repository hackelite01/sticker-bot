package main

import (
	"archive/zip"
	"bytes"
	"crypto/md5"
	"encoding/json"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	"image/png"
	"strings"
	"time"

	_ "golang.org/x/image/bmp"
	_ "golang.org/x/image/riff"
	_ "golang.org/x/image/tiff"
	_ "golang.org/x/image/vp8"
	_ "golang.org/x/image/vp8l"
	_ "golang.org/x/image/webp"

	"log"

	"github.com/hackelite01/sticker-bot/internal/emoji"
	"github.com/hackelite01/sticker-bot/internal/imaging"
	telegram "github.com/hackelite01/sticker-bot/internal/telebot.v2"
)

type  struct {
	*telegram.Bot
	Tick <-chan time.Time
}

func (bot *stickerBot) init() {
	bot.Handle("/start", bot.commandStart)
	bot.Handle("/help", bot.commandHelp)
	bot.Handle("/clone", bot.commandClone)
	bot.Handle("/steal", bot.commandSteal)
	bot.Handle("/clear", bot.commandClear)
	bot.Handle("/list", bot.commandList)
	bot.Handle("/zip", bot.commandZip)
	bot.Handle(telegram.OnSticker, printAndHandleMessage(bot.handleMessage))
	bot.Handle(telegram.OnPhoto, printAndHandleMessage(bot.handleMessage))
	bot.Handle(telegram.OnDocument, printAndHandleMessage(bot.handleMessage))
	bot.Handle(telegram.OnCallback, func(c *telegram.Callback) {
		i := strings.IndexRune(c.Data, '|')
		data := c.Data[i+1:]
		if err := bot.deleteStickerFromSet(data); err != nil {
			log.Printf("callback: %v", err)
			bot.replyWithHelp(c.Message, fmt.Sprintf("ERROR: %v", err), telegram.Silent)
		}
		bot.replyWithHelp(c.Message, "removed sticker from its set", telegram.Silent)
	})
	bot.Handle(telegram.OnText, func(m *telegram.Message) {
		if isStickerSetURL(m.Text) {
			m.Payload = m.Text
			bot.commandSteal(m)
		}
		for _, entity := range m.Entities {
			if isStickerSetURL(entity.URL) {
				m.Payload = entity.URL
				bot.commandSteal(m)
			}
		}
	})

	// fallback action: just print (if verbose=true)
	bot.Handle(telegram.OnAddedToGroup, printAndHandleMessage(nil))
	bot.Handle(telegram.OnAudio, printAndHandleMessage(nil))
	bot.Handle(telegram.OnChannelPost, printAndHandleMessage(nil))
	bot.Handle(telegram.OnCheckout, printAndHandleMessage(nil))
	bot.Handle(telegram.OnChosenInlineResult, printAndHandleMessage(nil))
	bot.Handle(telegram.OnContact, printAndHandleMessage(nil))
	bot.Handle(telegram.OnEdited, printAndHandleMessage(nil))
	bot.Handle(telegram.OnEditedChannelPost, printAndHandleMessage(nil))
	bot.Handle(telegram.OnGroupPhotoDeleted, printAndHandleMessage(nil))
	bot.Handle(telegram.OnLocation, printAndHandleMessage(nil))
	bot.Handle(telegram.OnMigration, printAndHandleMessage(nil))
	bot.Handle(telegram.OnNewGroupPhoto, printAndHandleMessage(nil))
	bot.Handle(telegram.OnNewGroupTitle, printAndHandleMessage(nil))
	bot.Handle(telegram.OnPinned, printAndHandleMessage(nil))
	bot.Handle(telegram.OnQuery, printAndHandleMessage(nil))
	bot.Handle(telegram.OnUserJoined, printAndHandleMessage(nil))
	bot.Handle(telegram.OnUserLeft, printAndHandleMessage(nil))
	bot.Handle(telegram.OnVenue, printAndHandleMessage(nil))
	bot.Handle(telegram.OnVideo, printAndHandleMessage(nil))
	bot.Handle(telegram.OnVideoNote, printAndHandleMessage(nil))
	bot.Handle(telegram.OnVoice, printAndHandleMessage(nil))
}

func (bot *stickerBot) stolenStickerSetName(u *telegram.User) string {
	return bot.stickerSetName(fmt.Sprintf("%x", md5.Sum([]byte(u.Recipient()))))
}

func (bot *stickerBot) stickerSetName(prefix string) string {
	me := bot.Me.Username
	name := fmt.Sprintf("x%s_by_%s", prefix, me)
	if len(name) > 64 {
		diff := len(name) - 64
		name = fmt.Sprintf("x%s_by_%s", prefix[:len(prefix)-diff], me)
	}
	return name
}

type createNewStickerSetRequest struct {
	UserID     string `json:"user_id"`
	Name       string `json:"name"`
	Title      string `json:"title"`
	PNGSticker string `json:"png_sticker"`
	Emojis     string `json:"emojis"`
}

type errorReply struct {
	OK          bool        `json:"ok"`
	ErrorCode   int         `json:"error_code"`
	Description string      `json:"description"`
	Result      interface{} `json:"result"`
}

func (e errorReply) Error() string {
	return fmt.Sprintf("%v (%s)", e.ErrorCode, e.Description)
}

type stickerSetReply struct {
	Name          string             `json:"name"`
	Title         string             `json:"title"`
	IsAnimated    bool               `json:"is_animated,omitempty"`
	ContainsMasks bool               `json:"contains_masks,omitempty"`
	Stickers      []telegram.Sticker `json:"stickers"`
}

func stickerSetURL(name string) string {
	return fmt.Sprintf("t.me/addstickers/%s", name)
}

func (bot *stickerBot) commandStart(m *telegram.Message) {
	<-bot.Tick
	name := bot.stolenStickerSetName(m.Sender)
	if _, err := bot.getStickerSet(name); err == nil {
		log.Printf("commandStart: sticker set %q already exists, skipping creation", name)
		return
	}
	initialSticker, _, _ := image.Decode(bytes.NewReader(initialStickerBytes))
	url, err := bot.createNewStickerSet(m.Sender.Recipient(), name, initialSticker)
	if err != nil {
		log.Printf("commandStart: %v", err)
		return
	}
	bot.replyWithHelp(m, url, telegram.Silent)
}

func (bot *stickerBot) commandClone(m *telegram.Message) {
	name := stickerSetNameOfURL(m.Payload)
	if m.Payload == "" {
		name = bot.stolenStickerSetName(m.Sender)
	}
	stickerSet, err := bot.getStickerSet(name)
	if err != nil {
		log.Printf("commandClone: %v", err)
		bot.replyWithHelp(m, fmt.Sprintf("ERROR: sticker set `%s` not found", name))
	}
	if len(stickerSet.Stickers) == 0 {
		bot.replyWithHelp(m, fmt.Sprintf("sticker set `%s` is empty. not cloning.", name))
		return
	}
	cloneNameHash := md5.Sum([]byte(fmt.Sprintf("%v%v%v", m.Unixtime, m.Sender.Recipient(), m.ID)))
	cloneName := bot.stickerSetName(fmt.Sprintf("%x", cloneNameHash))
	coverReader, err := bot.GetFile(&stickerSet.Stickers[0].File)
	if err != nil {
		log.Printf("commandClone: %v", err)
		return
	}
	cover, _, _ := image.Decode(coverReader)
	cloneURL, err := bot.createNewStickerSet(m.Sender.Recipient(), cloneName, cover)
	if err != nil {
		log.Printf("commandClone: %v", err)
		return
	}
	for _, sticker := range stickerSet.Stickers[1:] {
		bot.addStickerToSet(m, telegram.Sticker{
			File:    sticker.File,
			Emoji:   sticker.Emoji,
			SetName: cloneName,
		})
	}
	reply, err := bot.replyWithHelp(m, "created clone: "+cloneURL, telegram.Silent)
	if err != nil {
		log.Printf("commandClone: %v", err)
		return
	}
	jsonOut.Encode(reply)
}

func (bot *stickerBot) commandSteal(m *telegram.Message) {
	if m.Payload == "" {
		bot.replyWithHelp(m, fmt.Sprintf("ERROR: sticker set not specified"))
	}
	fromName := stickerSetNameOfURL(m.Payload)
	toName := bot.stolenStickerSetName(m.Sender)
	sourceStickerSet, err := bot.getStickerSet(fromName)
	if err != nil {
		log.Printf("commandSteal: %v", err)
		bot.replyWithHelp(m, fmt.Sprintf("ERROR: sticker set `%s` not found", fromName))
	}
	if len(sourceStickerSet.Stickers) == 0 {
		bot.replyWithHelp(m, fmt.Sprintf("sticker set `%s` is empty. not stealing.", fromName))
		return
	}
	for _, sticker := range sourceStickerSet.Stickers {
		bot.addStickerToSet(m, telegram.Sticker{
			File:    sticker.File,
			Emoji:   sticker.Emoji,
			SetName: toName,
		})
	}
	reply, err := bot.replyWithHelp(m, "added all stickers to scratchpad.", telegram.Silent)
	if err != nil {
		log.Printf("commandSteal: %v", err)
		return
	}
	jsonOut.Encode(reply)
}

func isStickerSetURL(u string) bool {
	switch {
	case strings.HasPrefix(u, "https://t.me/addstickers/"),
		strings.HasPrefix(u, "http://t.me/addstickers/"),
		strings.HasPrefix(u, "t.me/addstickers/"):
		return true
	default:
		return false
	}
}
func stickerSetNameOfURL(u string) string {
	u = strings.TrimPrefix(u, "https://")
	u = strings.TrimPrefix(u, "http://")
	u = strings.TrimPrefix(u, "t.me/addstickers/")
	return u
}

func (bot *stickerBot) commandZip(m *telegram.Message) {
	name := stickerSetNameOfURL(m.Payload)
	if name == "" {
		name = bot.stolenStickerSetName(m.Sender)
	}
	stickerSet, err := bot.getStickerSet(name)
	if err != nil {
		log.Printf("commandZip: %v", err)
		bot.replyWithHelp(m, fmt.Sprintf("ERROR: sticker set `%s` not found", name))
		return
	}
	var buf bytes.Buffer
	zipWriter := zip.NewWriter(&buf)
	for i, sticker := range stickerSet.Stickers {
		reader, err := bot.GetFile(&sticker.File)
		if err != nil {
			log.Printf("commandZip: %v", err)
			continue
		}
		image, _, err := image.Decode(reader)
		if err != nil {
			log.Printf("commandZip: %v", err)
			continue
		}
		w, err := zipWriter.Create(fmt.Sprintf("sticker-%03d.png", i))
		if err != nil {
			log.Printf("commandZip: %v", err)
			continue
		}
		if err := png.Encode(w, image); err != nil {
			log.Printf("commandZip: %v", err)
			continue
		}
		reader.Close()
	}
	if err := zipWriter.Close(); err != nil {
		log.Printf("commandZip: %v", err)
	}
	fileName := fmt.Sprintf("%s.zip", stickerSet.Name)
	reply, err := bot.Reply(m, &telegram.Document{File: telegram.FromReader(&buf), FileName: fileName, MIME: "application/zip", Caption: stickerSet.Title})
	if err != nil {
		log.Printf("commandZip: %v", err)
	}
	jsonOut.Encode(reply)
}

func (bot *stickerBot) commandClear(m *telegram.Message) {
	name := bot.stolenStickerSetName(m.Sender)
	stickerSet, err := bot.getStickerSet(name)
	if err != nil {
		log.Printf("commandClear: %v", err)
	}
	for _, sticker := range stickerSet.Stickers {
		bot.deleteStickerFromSet(sticker.FileID)
	}
	bot.replyWithHelp(m, "cleared "+stickerSetURL(name), telegram.Silent)
}

func (bot *stickerBot) replyWithHelp(m *telegram.Message, text string, options ...interface{}) (*telegram.Message, error) {
	return bot.Reply(m, text+"\n/help", options...)
}

func (bot *stickerBot) commandHelp(m *telegram.Message) {
	bot.Reply(m, `/help
Send/forward stickers and sticker set URLs to me to add them to your scratchpad sticker set!

/start - Create your scratchpad sticker set
/list  - List scratchpad stickers
/clear - Clear scratchpad sticker set
/clone [STICKER_SET] - Make a permanent clone of the scratchpad sticker set, or the specified sticker set
/steal STICKER_SET - Add all stickers from this set to the scratchpad sticker set
/zip [STICKER_SET] - Download the scratchpad sticker set, or the specified sticker set as a zip archive`)
}

func (bot *stickerBot) commandList(m *telegram.Message) {
	name := bot.stolenStickerSetName(m.Sender)
	stickerSet, err := bot.getStickerSet(name)
	if err != nil {
		log.Printf("commandList: %v", err)
	}
	for _, sticker := range stickerSet.Stickers {
		reply, err := bot.sendSticker(m.Sender, sticker.FileID, m.ID)
		if err != nil {
			log.Printf("commandList: reply: %v", err)
		}
		jsonOut.Encode(reply)
	}
}

func printAndHandleMessage(f func(*telegram.Message)) func(*telegram.Message) {
	return func(m *telegram.Message) {
		jsonOut.Encode(m)
		if f == nil {
			return
		}
		f(m)
	}
}

func (bot *stickerBot) handleCallback(m *telegram.Callback) {
	jsonOut.Encode(m)
}

func (bot *stickerBot) handleMessage(m *telegram.Message) {
	if m.Sender.ID == bot.Me.ID {
		return
	}
	if m.Sticker != nil {
		<-bot.Tick
		bot.commandStart(m)
		m.Sticker.SetName = bot.stolenStickerSetName(m.Sender)
		bot.addStickerToSet(m, *m.Sticker)
	}
	if m.Document != nil {
		<-bot.Tick
		bot.commandStart(m)
		bot.addDocumentToSet(m, *m.Document)
	}
	if m.Photo != nil {
		<-bot.Tick
		bot.commandStart(m)
		bot.addPhotoToSet(m, *m.Photo)
	}
}

type addStickerToSet struct {
	UserID     string `json:"user_id"`
	Name       string `json:"name"`
	PNGSticker string `json:"png_sticker"`
	Emojis     string `json:"emojis"`
}

func (bot *stickerBot) addDocumentToSet(m *telegram.Message, document telegram.Document) {
	emojis := findEmoji(m.Text)
	if emojis == "" {
		emojis = config.DefaultEmoji
	}
	bot.addStickerToSet(m, telegram.Sticker{
		File:    document.File,
		Emoji:   emojis,
		SetName: bot.stolenStickerSetName(m.Sender),
	})
}

func (bot *stickerBot) addPhotoToSet(m *telegram.Message, photo telegram.Photo) {
	emojis := findEmoji(m.Text)
	if emojis == "" {
		emojis = config.DefaultEmoji
	}
	bot.addStickerToSet(m, telegram.Sticker{
		File:    photo.File,
		Emoji:   emojis,
		SetName: bot.stolenStickerSetName(m.Sender),
	})
}

func (bot *stickerBot) addStickerToSet(m *telegram.Message, sticker telegram.Sticker) {
	setName := sticker.SetName
	fileReader, err := bot.GetFile(&sticker.File)
	if err != nil {
		log.Printf("addStickerToSet: get file: %v", err)
		return
	}
	image, _, err := image.Decode(fileReader)
	if err != nil {
		log.Printf("addStickerToSet: decode image: %v", err)
		return
	}
	image = imaging.ResizeTarget(image, 512, 512)
	var buf bytes.Buffer
	if err := png.Encode(&buf, image); err != nil {
		log.Printf("addStickerToSet: encode png: %v", err)
		return
	}
	file, err := bot.uploadStickerFile(m.Sender.Recipient(), telegram.FromReader(&buf))
	if err != nil {
		log.Printf("addStickerToSet: upload png: %v", err)
	}
	resp, err := bot.Raw("addStickerToSet", addStickerToSet{
		UserID:     m.Sender.Recipient(),
		Name:       setName,
		PNGSticker: file.FileID,
		Emojis:     sticker.Emoji,
	})
	if err != nil {
		log.Printf("addStickerToSet: %v", err)
		return
	}
	var raw interface{}
	json.Unmarshal(resp, &raw)
	jsonOut.Encode(raw)
	fileID := file.FileID
	for i := 0; i < config.MaxRetries; i++ {
		<-bot.Tick
		stickerSet, err := bot.getStickerSet(setName)
		if err != nil {
			log.Printf("addStickerToSet: get %v", err)
			return
		}
		if len(stickerSet.Stickers) > 0 {
			uploaded := stickerSet.Stickers[len(stickerSet.Stickers)-1]
			fileID = uploaded.FileID
			break
		}
	}
	reply, err := bot.sendSticker(m.Sender, fileID, m.ID)
	if err != nil {
		log.Printf("addStickerToSet: reply: %v", err)
	}
	jsonOut.Encode(reply)
}

func findEmoji(s string) string {
	textEmoji := emoji.FindAll(s)
	var buf bytes.Buffer
	for _, r := range textEmoji {
		if e, ok := r.Match.(emoji.Emoji); ok {
			(&buf).Write([]byte(e.Value))
		}
	}
	return buf.String()
}
