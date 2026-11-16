package xray

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/xtls/xray-core/app/proxyman"
	"github.com/xtls/xray-core/common/serial"
	"github.com/xtls/xray-core/core"
	"google.golang.org/protobuf/encoding/protowire"
)

// Accept Payloads is a server-only extension to the pinned stream settings.
// Stock panel-side Xray types do not know field 13, so preserve it explicitly
// when serializing a live AddInbound request to the bundled patched core.
func applyPayloadToAPI(config *core.InboundHandlerConfig, enabled bool) error {
	if !enabled {
		return nil
	}
	instance, err := config.ReceiverSettings.GetInstance()
	if err != nil {
		return err
	}
	receiver, ok := instance.(*proxyman.ReceiverConfig)
	if !ok || receiver.StreamSettings == nil {
		return errors.New("missing payload stream configuration")
	}
	stream := receiver.StreamSettings.ProtoReflect()
	unknown := protowire.AppendTag(stream.GetUnknown(), 13, protowire.VarintType)
	stream.SetUnknown(protowire.AppendVarint(unknown, 1))
	config.ReceiverSettings = serial.ToTypedMessage(receiver)
	return nil
}

func InjectPayloadEnabled(stream []byte) (bool, error) {
	if len(bytes.TrimSpace(stream)) == 0 || bytes.Equal(bytes.TrimSpace(stream), []byte("null")) {
		return false, nil
	}
	var settings struct {
		AcceptPayloads *bool  `json:"acceptPayloads"`
		Network        string `json:"network"`
		TLS            struct {
			ALPN []string `json:"alpn"`
		} `json:"tlsSettings"`
		TCP struct {
			InjectPayload bool `json:"injectPayload"`
			Header        struct {
				Type string `json:"type"`
			} `json:"header"`
		} `json:"tcpSettings"`
		RAW struct {
			InjectPayload bool `json:"injectPayload"`
			Header        struct {
				Type string `json:"type"`
			} `json:"header"`
		} `json:"rawSettings"`
	}
	if err := json.Unmarshal(stream, &settings); err != nil {
		return false, fmt.Errorf("invalid stream settings: %w", err)
	}
	enabled := settings.TCP.InjectPayload || settings.RAW.InjectPayload
	if settings.AcceptPayloads != nil {
		enabled = *settings.AcceptPayloads
	}
	if !enabled {
		return false, nil
	}
	switch settings.Network {
	case "", "tcp", "raw", "ws", "websocket", "httpupgrade", "grpc", "xhttp", "splithttp":
	default:
		return false, errors.New("the Accept Payloads option supports TCP, WebSocket, HTTPUpgrade, gRPC and XHTTP over TCP")
	}
	if (settings.Network == "xhttp" || settings.Network == "splithttp") && len(settings.TLS.ALPN) == 1 && settings.TLS.ALPN[0] == "h3" {
		return false, errors.New("HTTP payloads require XHTTP over TCP (HTTP/1.1 or HTTP/2), not QUIC")
	}
	for _, kind := range []string{settings.TCP.Header.Type, settings.RAW.Header.Type} {
		if kind != "" && kind != "none" {
			return false, errors.New("the Accept Payloads option cannot be combined with HTTP obfuscation")
		}
	}
	return true, nil
}
