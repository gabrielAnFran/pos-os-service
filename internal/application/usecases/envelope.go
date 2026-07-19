package usecases

import (
	"encoding/json"

	"github.com/gabrielAnFran/pos-os-service/internal/infrastructure/messaging"
)

// marshalEnvelope serializes the full event envelope so the outbox dispatcher
// can unmarshal it back into messaging.Event and publish it verbatim.
func marshalEnvelope(ev messaging.Event) ([]byte, error) {
	return json.Marshal(ev)
}
