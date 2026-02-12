package qlab

import (
	"encoding/json"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

const (
	DefaultPort = 53000

	slipEnd    = 0xC0
	slipEsc    = 0xDB
	slipEscEnd = 0xDC
	slipEscEsc = 0xDD
)

type Workspace struct {
	DisplayName string `json:"displayName"`
	UniqueID    string `json:"uniqueID"`
	HasPasscode bool   `json:"hasPasscode"`
}

type Cue struct {
	UniqueID  string `json:"uniqueID"`
	Number    string `json:"number"`
	Name      string `json:"name"`
	ListName  string `json:"listName"`
	Type      string `json:"type"`
	ColorName string `json:"colorName"`
	Flagged   bool   `json:"flagged"`
	Armed     bool   `json:"armed"`
	Cues      []Cue  `json:"cues"`
}

type Reply struct {
	WorkspaceID string          `json:"workspace_id"`
	Address     string          `json:"address"`
	Status      string          `json:"status"`
	Data        json.RawMessage `json:"data"`
}

type Update struct {
	Address string
}

type Client struct {
	conn    net.Conn
	mu      sync.Mutex
	pending map[string]chan *Reply
	idSeq   atomic.Uint64
	updates chan Update
}

func Dial(host string, port int) (*Client, error) {
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("%s:%d", host, port), 5*time.Second)
	if err != nil {
		return nil, err
	}
	c := &Client{
		conn:    conn,
		pending: make(map[string]chan *Reply),
		updates: make(chan Update, 64),
	}
	go c.readLoop()
	return c, nil
}

func (c *Client) Close() error {
	return c.conn.Close()
}

func (c *Client) Updates() <-chan Update {
	return c.updates
}

func (c *Client) readLoop() {
	buf := make([]byte, 0, 65536)
	tmp := make([]byte, 4096)
	for {
		n, err := c.conn.Read(tmp)
		if err != nil {
			return
		}
		buf = append(buf, tmp[:n]...)
		for {
			frame, rest, ok := extractSLIPFrame(buf)
			if !ok {
				break
			}
			buf = rest
			c.handleFrame(frame)
		}
	}
}

func extractSLIPFrame(data []byte) (frame []byte, rest []byte, ok bool) {
	start := -1
	for i, b := range data {
		if b == slipEnd {
			if start == -1 {
				start = i
			} else {
				raw := data[start+1 : i]
				frame := slipDecode(raw)
				return frame, data[i+1:], true
			}
		}
	}
	return nil, data, false
}

func slipEncode(data []byte) []byte {
	out := []byte{slipEnd}
	for _, b := range data {
		switch b {
		case slipEnd:
			out = append(out, slipEsc, slipEscEnd)
		case slipEsc:
			out = append(out, slipEsc, slipEscEsc)
		default:
			out = append(out, b)
		}
	}
	out = append(out, slipEnd)
	return out
}

func slipDecode(data []byte) []byte {
	out := make([]byte, 0, len(data))
	for i := 0; i < len(data); i++ {
		if data[i] == slipEsc && i+1 < len(data) {
			switch data[i+1] {
			case slipEscEnd:
				out = append(out, slipEnd)
			case slipEscEsc:
				out = append(out, slipEsc)
			}
			i++
		} else {
			out = append(out, data[i])
		}
	}
	return out
}

func (c *Client) handleFrame(frame []byte) {
	addr, args, err := parseOSC(frame)
	if err != nil {
		return
	}

	if len(addr) > 8 && addr[:8] == "/update/" {
		c.handleUpdate(addr)
		return
	}

	if len(addr) > 7 && addr[:7] == "/reply/" {
		if len(args) == 0 {
			return
		}
		jsonStr, ok := args[0].(string)
		if !ok {
			return
		}
		var reply Reply
		if err := json.Unmarshal([]byte(jsonStr), &reply); err != nil {
			return
		}
		replyAddr := addr[6:]
		c.mu.Lock()
		ch, exists := c.pending[replyAddr]
		if exists {
			delete(c.pending, replyAddr)
		}
		c.mu.Unlock()
		if exists {
			ch <- &reply
		}
	}
}

func (c *Client) handleUpdate(addr string) {
	select {
	case c.updates <- Update{Address: addr}:
	default:
	}
}

func (c *Client) send(addr string, args ...any) error {
	msg := buildOSC(addr, args...)
	encoded := slipEncode(msg)
	c.mu.Lock()
	defer c.mu.Unlock()
	_, err := c.conn.Write(encoded)
	return err
}

func (c *Client) sendAndWait(addr string, timeout time.Duration, args ...any) (*Reply, error) {
	ch := make(chan *Reply, 1)
	c.mu.Lock()
	c.pending[addr] = ch
	c.mu.Unlock()

	if err := c.send(addr, args...); err != nil {
		c.mu.Lock()
		delete(c.pending, addr)
		c.mu.Unlock()
		return nil, err
	}

	select {
	case reply := <-ch:
		if reply.Status != "ok" {
			return reply, fmt.Errorf("qlab: %s: %s", addr, reply.Status)
		}
		return reply, nil
	case <-time.After(timeout):
		c.mu.Lock()
		delete(c.pending, addr)
		c.mu.Unlock()
		return nil, fmt.Errorf("qlab: %s: timeout", addr)
	}
}

func (c *Client) request(addr string, args ...any) (*Reply, error) {
	return c.sendAndWait(addr, 5*time.Second, args...)
}

func (c *Client) Version() (string, error) {
	reply, err := c.request("/version")
	if err != nil {
		return "", err
	}
	var v string
	if err := json.Unmarshal(reply.Data, &v); err != nil {
		return "", err
	}
	return v, nil
}

func (c *Client) Workspaces() ([]Workspace, error) {
	reply, err := c.request("/workspaces")
	if err != nil {
		return nil, err
	}
	var ws []Workspace
	if err := json.Unmarshal(reply.Data, &ws); err != nil {
		return nil, err
	}
	return ws, nil
}

func (c *Client) Connect(workspaceID string, passcode string) error {
	addr := fmt.Sprintf("/workspace/%s/connect", workspaceID)
	if passcode != "" {
		_, err := c.request(addr, passcode)
		return err
	}
	_, err := c.request(addr)
	return err
}

func (c *Client) Disconnect(workspaceID string) error {
	return c.send(fmt.Sprintf("/workspace/%s/disconnect", workspaceID))
}

func (c *Client) AlwaysReply(workspaceID string, enable bool) error {
	v := int32(0)
	if enable {
		v = 1
	}
	return c.send(fmt.Sprintf("/workspace/%s/alwaysReply", workspaceID), v)
}

func (c *Client) EnableUpdates(workspaceID string, enable bool) error {
	v := int32(0)
	if enable {
		v = 1
	}
	return c.send(fmt.Sprintf("/workspace/%s/updates", workspaceID), v)
}

func (c *Client) Thump(workspaceID string) error {
	return c.send(fmt.Sprintf("/workspace/%s/thump", workspaceID))
}

func (c *Client) Go(workspaceID string) error {
	return c.send(fmt.Sprintf("/workspace/%s/go", workspaceID))
}

func (c *Client) GoTo(workspaceID string, cueNumber string) error {
	return c.send(fmt.Sprintf("/workspace/%s/go", workspaceID), cueNumber)
}

func (c *Client) Stop(workspaceID string) error {
	return c.send(fmt.Sprintf("/workspace/%s/stop", workspaceID))
}

func (c *Client) Pause(workspaceID string) error {
	return c.send(fmt.Sprintf("/workspace/%s/pause", workspaceID))
}

func (c *Client) Resume(workspaceID string) error {
	return c.send(fmt.Sprintf("/workspace/%s/resume", workspaceID))
}

func (c *Client) Panic(workspaceID string) error {
	return c.send(fmt.Sprintf("/workspace/%s/panic", workspaceID))
}

func (c *Client) Reset(workspaceID string) error {
	return c.send(fmt.Sprintf("/workspace/%s/reset", workspaceID))
}

func (c *Client) CueLists(workspaceID string) ([]Cue, error) {
	addr := fmt.Sprintf("/workspace/%s/cueLists", workspaceID)
	reply, err := c.request(addr)
	if err != nil {
		return nil, err
	}
	var cues []Cue
	if err := json.Unmarshal(reply.Data, &cues); err != nil {
		return nil, err
	}
	return cues, nil
}

func (c *Client) SelectedCues(workspaceID string) ([]Cue, error) {
	addr := fmt.Sprintf("/workspace/%s/selectedCues", workspaceID)
	reply, err := c.request(addr)
	if err != nil {
		return nil, err
	}
	var cues []Cue
	if err := json.Unmarshal(reply.Data, &cues); err != nil {
		return nil, err
	}
	return cues, nil
}

func (c *Client) RunningCues(workspaceID string) ([]Cue, error) {
	addr := fmt.Sprintf("/workspace/%s/runningCues", workspaceID)
	reply, err := c.request(addr)
	if err != nil {
		return nil, err
	}
	var cues []Cue
	if err := json.Unmarshal(reply.Data, &cues); err != nil {
		return nil, err
	}
	return cues, nil
}

func (c *Client) CueGet(workspaceID string, cueID string, property string) (*Reply, error) {
	addr := fmt.Sprintf("/workspace/%s/cue_id/%s/%s", workspaceID, cueID, property)
	return c.request(addr)
}

func (c *Client) CueGetByNumber(workspaceID string, cueNumber string, property string) (*Reply, error) {
	addr := fmt.Sprintf("/workspace/%s/cue/%s/%s", workspaceID, cueNumber, property)
	return c.request(addr)
}

func (c *Client) CueSet(workspaceID string, cueID string, property string, value any) error {
	addr := fmt.Sprintf("/workspace/%s/cue_id/%s/%s", workspaceID, cueID, property)
	return c.send(addr, value)
}

func (c *Client) CueSetByNumber(workspaceID string, cueNumber string, property string, value any) error {
	addr := fmt.Sprintf("/workspace/%s/cue/%s/%s", workspaceID, cueNumber, property)
	return c.send(addr, value)
}

func (c *Client) CueStart(workspaceID string, cueID string) error {
	return c.send(fmt.Sprintf("/workspace/%s/cue_id/%s/start", workspaceID, cueID))
}

func (c *Client) CueStop(workspaceID string, cueID string) error {
	return c.send(fmt.Sprintf("/workspace/%s/cue_id/%s/stop", workspaceID, cueID))
}

func (c *Client) CuePause(workspaceID string, cueID string) error {
	return c.send(fmt.Sprintf("/workspace/%s/cue_id/%s/pause", workspaceID, cueID))
}

func (c *Client) CueResume(workspaceID string, cueID string) error {
	return c.send(fmt.Sprintf("/workspace/%s/cue_id/%s/resume", workspaceID, cueID))
}

func (c *Client) CueLoad(workspaceID string, cueID string) error {
	return c.send(fmt.Sprintf("/workspace/%s/cue_id/%s/load", workspaceID, cueID))
}

func (c *Client) CueReset(workspaceID string, cueID string) error {
	return c.send(fmt.Sprintf("/workspace/%s/cue_id/%s/reset", workspaceID, cueID))
}
