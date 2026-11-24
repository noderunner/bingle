package store

import (
	"context"
	"fmt"
	"strings"
)

// ChatTranslation is a chat line in one reader's language (ADR 0143). An empty
// Body means none is needed: the line is in that language already, has nothing
// to translate, or the answer was dropped.
type ChatTranslation struct {
	Source string
	Body   string
}

// chatTranslationTable is the table of a game's chat's translations, or of the
// global chat's.
func chatTranslationTable(global bool) string {
	if global {
		return "global_chat_translations"
	}
	return "chat_translations"
}

// AddChatTranslation keeps what a chat line came to in a language. A line
// removed meanwhile keeps nothing, and that is no error.
func (s *Store) AddChatTranslation(ctx context.Context, global bool, messageID int64, lang string, t ChatTranslation) error {
	table := chatTranslationTable(global)
	_, err := s.exec(ctx, "AddChatTranslation."+table,
		`INSERT INTO `+table+` (message_id, lang, source, body)
		 SELECT ?, ?, ?, ? WHERE EXISTS (SELECT 1 FROM `+parentOf(global)+` WHERE id = ?)
		 ON CONFLICT (message_id, lang) DO UPDATE SET source = excluded.source, body = excluded.body`,
		messageID, lang, t.Source, t.Body, messageID)
	if err != nil {
		return fmt.Errorf("keeping a chat translation (message %d): %w", messageID, err)
	}
	return nil
}

func parentOf(global bool) string {
	if global {
		return "global_chat"
	}
	return "chat_messages"
}

// ChatTranslations are what the lines ids came to in lang, by line; a line
// never asked for in lang is not in it.
func (s *Store) ChatTranslations(ctx context.Context, global bool, ids []int64, lang string) (map[int64]ChatTranslation, error) {
	out := map[int64]ChatTranslation{}
	if len(ids) == 0 {
		return out, nil
	}
	table := chatTranslationTable(global)
	args := make([]any, 0, len(ids)+1)
	args = append(args, lang)
	for _, id := range ids {
		args = append(args, id)
	}
	rows, err := s.query(ctx, "ChatTranslations."+table,
		`SELECT message_id, source, body FROM `+table+` WHERE lang = ? AND message_id IN (?`+strings.Repeat(", ?", len(ids)-1)+`)`, args...)
	if err != nil {
		return nil, fmt.Errorf("reading chat translations: %w", err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var id int64
		var t ChatTranslation
		if err := rows.Scan(&id, &t.Source, &t.Body); err != nil {
			return nil, err
		}
		out[id] = t
	}
	return out, rows.Err()
}
