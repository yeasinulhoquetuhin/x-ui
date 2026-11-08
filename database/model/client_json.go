package model

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// UnmarshalJSON accepts both the panel's string-valued Telegram field and
// numeric IDs from API clients/older databases. Other client fields retain
// their normal types, and a failed decode never partially replaces the client.
func (c *Client) UnmarshalJSON(data []byte) error {
	type clientAlias Client
	var parsed Client
	raw := struct {
		*clientAlias
		TelegramID json.RawMessage `json:"tgId"`
	}{clientAlias: (*clientAlias)(&parsed)}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	id := bytes.TrimSpace(raw.TelegramID)
	if len(id) > 0 && !bytes.Equal(id, []byte("null")) {
		if id[0] == '"' {
			var text string
			if err := json.Unmarshal(id, &text); err != nil {
				return err
			}
			text = strings.TrimSpace(text)
			if text != "" {
				value, err := strconv.ParseInt(text, 10, 64)
				if err != nil {
					return fmt.Errorf("invalid client tgId: %w", err)
				}
				parsed.TgID = value
			}
		} else if err := json.Unmarshal(id, &parsed.TgID); err != nil {
			return fmt.Errorf("invalid client tgId: %w", err)
		}
	}
	*c = parsed
	return nil
}
