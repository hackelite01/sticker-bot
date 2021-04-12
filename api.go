package main

import (
	"bytes"
	"crypto/md5"
	"encoding/json"
	"fmt"
	"image"
	"image/png"

	"github.com/hackelite01/sticker-bot/internal/imaging"

	telegram "github.com/hackelite01/sticker-bot/internal/telebot.v2"
)

func (bot *stickerBot) deleteStickerFromSet(stickerFileID string) error {
	type deleteStickerFromSetRequest struct {
		Sticker string `json:"sticker"`
	}
	resp, err := bot.Raw("deleteStickerFromSet", deleteStickerFromSetRequest{Sticker: stickerFileID})
	if err != nil {
		return fmt.Errorf("deleteStickerFromSet: %v", err)
	}
	var raw interface{}
	json.Unmarshal(resp, &raw)
	jsonOut.Encode(raw)
	respErr := errorReply{OK: true}
	err = json.Unmarshal(resp, &respErr)
	if err != nil {
		return fmt.Errorf("deleteStickerFromSet: %v", err)
	}
	if !respErr.OK {
		return fmt.Errorf("deleteStickerFromSet: %v", respErr)
	}
	return nil
}

func (bot *stickerBot) sendSticker(to telegram.Recipient, stickerFileID string, replyToMessageID int) (*telegram.Message, error) {
	type sendStickerRequest struct {
		ChatID              string               `json:"chat_id"`
		Sticker             string               `json:"sticker"`
		DisableNotification bool                 `json:"disable_notification,omitempty"`
		ReplyToMessageID    int                  `json:"reply_to_message_id,omitempty"`
		ReplyMarkup         telegram.ReplyMarkup `json:"reply_markup"`
	}
	unique := fmt.Sprintf("%x", md5.Sum([]byte(stickerFileID+to.Recipient())))
	resp, err := bot.Raw("sendSticker", sendStickerRequest{
		ChatID:           to.Recipient(),
		Sticker:          stickerFileID,
		ReplyToMessageID: replyToMessageID,
		ReplyMarkup: telegram.ReplyMarkup{
			InlineKeyboard: [][]telegram.InlineButton{{
				{
					Unique: unique,
					Data:   stickerFileID,
					Text:   "Remove from set",
				},
			}},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("sendSticker: %v", err)
	}
	var raw interface{}
	json.Unmarshal(resp, &raw)
	jsonOut.Encode(raw)
	var out telegram.Message
	respErr := errorReply{OK: true, Result: out}
	err = json.Unmarshal(resp, &respErr)
	if err != nil {
		return nil, fmt.Errorf("sendSticker: %v", err)
	}
	if !respErr.OK {
		return nil, fmt.Errorf("sendSticker: %v", respErr)
	}
	return &out, nil
}

func (bot *stickerBot) getStickerSet(name string) (*stickerSetReply, error) {
	type getStickerSetRequest struct {
		Name string `json:"name"`
	}
	resp, err := bot.Raw("getStickerSet", getStickerSetRequest{Name: name})
	if err != nil {
		return nil, fmt.Errorf("getStickerSet: %v", err)
	}
	var raw interface{}
	json.Unmarshal(resp, &raw)
	jsonOut.Encode(raw)
	var out stickerSetReply
	respErr := errorReply{OK: true, Result: &out}
	err = json.Unmarshal(resp, &respErr)
	if err != nil {
		return nil, fmt.Errorf("getStickerSet: %v", err)
	}
	if !respErr.OK {
		return nil, fmt.Errorf("getStickerSet: %v", respErr)
	}
	return &out, nil
}

// Send delivers media through bot b to recipient.
func (bot *stickerBot) uploadStickerFile(userID string, f telegram.File) (*telegram.File, error) {
	params := map[string]string{
		"user_id": userID,
	}
	resp, err := bot.SendFiles("uploadStickerFile", map[string]telegram.File{"png_sticker": f}, params)
	if err != nil {
		return nil, fmt.Errorf("uploadStickerFile: %v", err)
	}
	var raw interface{}
	json.Unmarshal(resp, &raw)
	jsonOut.Encode(raw)
	var out telegram.File
	respErr := errorReply{OK: true, Result: &out}
	err = json.Unmarshal(resp, &respErr)
	if err != nil {
		return nil, fmt.Errorf("uploadStickerFile: %v", err)
	}
	if !respErr.OK {
		return nil, fmt.Errorf("uploadStickerFile: %v", respErr)
	}
	return &out, nil
}

func (bot *stickerBot) createNewStickerSet(userID, setName string, cover image.Image) (string, error) {
	cover = imaging.ResizeTarget(cover, 512, 512)
	var buf bytes.Buffer
	png.Encode(&buf, cover)
	file, err := bot.uploadStickerFile(userID, telegram.FromReader(&buf))
	if err != nil {
		return "", fmt.Errorf("createNewStickerSet: %v", err)
	}
	resp, err := bot.Raw("createNewStickerSet", createNewStickerSetRequest{
		UserID:     userID,
		Name:       setName,
		PNGSticker: file.FileID,
		Title:      config.DefaultEmoji,
		Emojis:     config.DefaultEmoji,
	})
	respErr := errorReply{OK: true}
	json.Unmarshal(resp, &respErr)
	if !respErr.OK {
		return "", fmt.Errorf("createNewStickerSet: %v", respErr)
	}
	return stickerSetURL(setName), nil
}
