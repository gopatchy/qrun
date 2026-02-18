package show

type StateType string

const (
	Lighting StateType = "lighting"
	Media    StateType = "media"
)

type State struct {
	ID       string    `json:"id"`
	Type     StateType `json:"type"`
	Sequence int       `json:"sequence"`
	Layer    int       `json:"layer"`

	LightingParams *LightingParams `json:"lightingParams,omitempty"`
	MediaParams    *MediaParams    `json:"mediaParams,omitempty"`
}

type LightingParams struct {
	Fixtures []FixtureSetting `json:"fixtures"`
}

type FixtureSetting struct {
	ID       string         `json:"id"`
	Channels map[string]int `json:"channels"`
}

type MediaParams struct {
	Source string `json:"source"`
	Loop   bool   `json:"loop"`
}

type Cue struct {
	ID       string `json:"id"`
	Sequence int    `json:"sequence"`
	Name     string `json:"name"`
}
