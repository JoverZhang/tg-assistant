package client

import (
	"github.com/gotd/td/tg"
)

func (c *Client) ForwardMedia(updates tg.UpdatesClass) error {
	// TODO: Implement this
	// sentMedias := extractSentMedias(updates)

	// // Forward the media to the target peer
	// targetPeer, err := c.ResolvePeer(-5054477506)
	// if err != nil {
	// 	return err
	// }
	// for _, media := range sentMedias {
	// 	if media.Photo != nil {
	// 		logger.Debug.Println("forwarding photo: ", media.Photo)
	// 		_, err = c.client.API().MessagesSendMedia(c.ctx, &tg.MessagesSendMediaRequest{
	// 			Peer:     targetPeer,
	// 			RandomID: randID(),
	// 			Media: &tg.InputMediaPhoto{
	// 				ID: &tg.InputPhoto{
	// 					ID:            media.Photo.ID,
	// 					AccessHash:    media.Photo.AccessHash,
	// 					FileReference: media.Photo.FileReference,
	// 				},
	// 			},
	// 		})
	// 		if err != nil {
	// 			return err
	// 		}
	// 	} else if media.Document != nil {
	// 		logger.Debug.Println("forwarding document: ", media.Document)

	// 		_, err = c.client.API().MessagesSendMedia(c.ctx, &tg.MessagesSendMediaRequest{
	// 			Peer:     targetPeer,
	// 			RandomID: randID(),
	// 			Media: &tg.InputMediaDocument{
	// 				ID: &tg.InputDocument{
	// 					ID:            media.Document.ID,
	// 					AccessHash:    media.Document.AccessHash,
	// 					FileReference: media.Document.FileReference,
	// 				},
	// 			},
	// 		})
	// 		if err != nil {
	// 			return err
	// 		}
	// 	} else {
	// 		logger.Debug.Println("unknown media type: ", media)
	// 	}
	// }

	return nil
}

func extractSentMedias(updates tg.UpdatesClass) []MediaHandle {
	var res []MediaHandle

	handleMsg := func(msg *tg.Message) {
		h := MediaHandle{
			MsgID:     msg.ID,
			GroupedID: msg.GroupedID,
		}

		switch m := msg.Media.(type) {
		case *tg.MessageMediaPhoto:
			if photo, ok := m.Photo.(*tg.Photo); ok {
				h.Photo = &tg.InputPhoto{
					ID:            photo.ID,
					AccessHash:    photo.AccessHash,
					FileReference: photo.FileReference,
				}
			}

		case *tg.MessageMediaDocument:
			if doc, ok := m.Document.(*tg.Document); ok {
				h.Document = &tg.InputDocument{
					ID:            doc.ID,
					AccessHash:    doc.AccessHash,
					FileReference: doc.FileReference,
				}
			}
		}

		if h.Photo != nil || h.Document != nil {
			res = append(res, h)
		}
	}

	switch u := updates.(type) {
	case *tg.Updates:
		for _, upd := range u.Updates {
			switch x := upd.(type) {
			case *tg.UpdateNewMessage:
				if msg, ok := x.Message.(*tg.Message); ok {
					handleMsg(msg)
				}
			case *tg.UpdateNewChannelMessage:
				if msg, ok := x.Message.(*tg.Message); ok {
					handleMsg(msg)
				}
			}
		}

	case *tg.UpdatesCombined:
		for _, upd := range u.Updates {
			switch x := upd.(type) {
			case *tg.UpdateNewMessage:
				if msg, ok := x.Message.(*tg.Message); ok {
					handleMsg(msg)
				}
			case *tg.UpdateNewChannelMessage:
				if msg, ok := x.Message.(*tg.Message); ok {
					handleMsg(msg)
				}
			}
		}
	}

	return res
}

type MediaHandle struct {
	MsgID     int
	GroupedID int64

	Photo    *tg.InputPhoto
	Document *tg.InputDocument
}
