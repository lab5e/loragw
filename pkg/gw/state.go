package gw

import (
	"bytes"
	"encoding/json"
	"os"
	"sync"
)

// GatewayState is a state object for the gateway. The state object keeps track of the
// device -> lora device ID mappings
type GatewayState struct {
	GatewayID  string            `json:"gatewayId"`
	LocalID    string            `json:"localId"`
	IDMappings map[string]string `json:"deviceMapping"`
	mutex      *sync.Mutex
}

// NewStateFromFile reads the state from a file. If the file name is empty or if the file doesn't exist an
// empty state struct will be returned
func NewStateFromFile(filename string) (*GatewayState, error) {
	ret := &GatewayState{
		IDMappings: make(map[string]string),
		mutex:      &sync.Mutex{},
	}

	if filename == "" {
		return ret, nil
	}
	buf, err := os.ReadFile(filename)
	if err != nil {
		if os.IsNotExist(err) {
			return ret, nil
		}
		return nil, err
	}
	if err := json.NewDecoder(bytes.NewBuffer(buf)).Decode(ret); err != nil {
		return nil, err
	}
	return ret, nil
}

// Save writes the state to a file if the file name is set.
func (g *GatewayState) Save(filename string) error {
	if filename == "" {
		return nil
	}
	g.mutex.Lock()
	defer g.mutex.Unlock()
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(g); err != nil {
		return err
	}
	return os.WriteFile(filename, buf.Bytes(), 0700)
}

// SetMapping sets the ID mapping for the Span device ID to the gateway internal ID
func (g *GatewayState) SetMapping(deviceID, otherID string) {
	g.mutex.Lock()
	defer g.mutex.Unlock()
	g.IDMappings[deviceID] = otherID
}

// GetMapping retrieves the mapping between the Span device ID and the internal ID
func (g *GatewayState) GetMapping(deviceID string) string {
	g.mutex.Lock()
	defer g.mutex.Unlock()
	return g.IDMappings[deviceID]
}

// GetReverseMapping returns the device ID corresponding to the local ID
func (g *GatewayState) GetReverseMapping(localID string) string {
	g.mutex.Lock()
	defer g.mutex.Unlock()
	for id, local := range g.IDMappings {
		if local == localID {
			return id
		}
	}
	return ""
}

// RemoveMapping removes a mapping between the Span device ID and the internal ID
func (g *GatewayState) RemoveMapping(id string) {
	g.mutex.Lock()
	defer g.mutex.Unlock()
	delete(g.IDMappings, id)
}
