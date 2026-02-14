package qlab

import (
	"encoding/json"
	"net"
	"strings"
	"sync"
)

type MockServer struct {
	listener net.Listener
	mu       sync.Mutex
	conns    []net.Conn

	Version    string
	Workspaces []Workspace
	CueLists   map[string][]Cue
}

func NewMockServer() (*MockServer, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	m := &MockServer{
		listener:   ln,
		Version:    "5.0.0",
		Workspaces: []Workspace{},
		CueLists:   make(map[string][]Cue),
	}
	go m.serve()
	return m, nil
}

func (m *MockServer) Port() int {
	return m.listener.Addr().(*net.TCPAddr).Port
}

func (m *MockServer) Close() error {
	err := m.listener.Close()
	m.mu.Lock()
	for _, conn := range m.conns {
		conn.Close()
	}
	m.mu.Unlock()
	return err
}

func (m *MockServer) SendUpdate(addr string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	msg := buildOSC(addr)
	encoded := slipEncode(msg)
	for _, conn := range m.conns {
		conn.Write(encoded)
	}
}

func (m *MockServer) serve() {
	for {
		conn, err := m.listener.Accept()
		if err != nil {
			return
		}
		m.mu.Lock()
		m.conns = append(m.conns, conn)
		m.mu.Unlock()
		go m.handleConn(conn)
	}
}

func (m *MockServer) handleConn(conn net.Conn) {
	buf := make([]byte, 0, 65536)
	tmp := make([]byte, 4096)
	for {
		n, err := conn.Read(tmp)
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
			addr, args, err := parseOSC(frame)
			if err != nil {
				continue
			}
			m.handleRequest(conn, addr, args)
		}
	}
}

func (m *MockServer) sendReply(conn net.Conn, addr string, wsID string, status string, data any) {
	jsonData, _ := json.Marshal(data)
	r := Reply{
		WorkspaceID: wsID,
		Address:     addr,
		Status:      status,
		Data:        json.RawMessage(jsonData),
	}
	replyJSON, _ := json.Marshal(r)
	msg := buildOSC("/reply"+addr, string(replyJSON))
	conn.Write(slipEncode(msg))
}

func (m *MockServer) handleRequest(conn net.Conn, addr string, args []any) {
	m.mu.Lock()
	defer m.mu.Unlock()

	switch {
	case addr == "/version":
		m.sendReply(conn, addr, "", "ok", m.Version)
		return
	case addr == "/workspaces":
		m.sendReply(conn, addr, "", "ok", m.Workspaces)
		return
	}

	parts := strings.SplitN(addr, "/", 5)
	if len(parts) < 4 || parts[1] != "workspace" {
		return
	}
	wsID := parts[2]
	rest := strings.Join(parts[3:], "/")

	switch {
	case rest == "connect":
		m.sendReply(conn, addr, wsID, "ok", "ok")
	case rest == "cueLists":
		cues := m.CueLists[wsID]
		if cues == nil {
			cues = []Cue{}
		}
		m.sendReply(conn, addr, wsID, "ok", cues)
	case rest == "selectedCues":
		m.sendReply(conn, addr, wsID, "ok", []Cue{})
	case rest == "runningCues":
		m.sendReply(conn, addr, wsID, "ok", []Cue{})
	case strings.HasPrefix(rest, "cue_id/"):
		sub := strings.SplitN(rest, "/", 3)
		if len(sub) < 3 {
			return
		}
		cue := m.findCueByID(wsID, sub[1])
		if cue == nil {
			m.sendReply(conn, addr, wsID, "not found", nil)
			return
		}
		if len(args) > 0 {
			m.setCueProperty(cue, sub[2], args[0])
		} else {
			m.sendReply(conn, addr, wsID, "ok", m.getCueProperty(cue, sub[2]))
		}
	case strings.HasPrefix(rest, "cue/"):
		sub := strings.SplitN(rest, "/", 3)
		if len(sub) < 3 {
			return
		}
		cue := m.findCueByNumber(wsID, sub[1])
		if cue == nil {
			m.sendReply(conn, addr, wsID, "not found", nil)
			return
		}
		if len(args) > 0 {
			m.setCueProperty(cue, sub[2], args[0])
		} else {
			m.sendReply(conn, addr, wsID, "ok", m.getCueProperty(cue, sub[2]))
		}
	default:
		m.sendReply(conn, addr, wsID, "ok", nil)
	}
}

func (m *MockServer) findCueByID(wsID, cueID string) *Cue {
	return findCueInList(m.CueLists[wsID], cueID, func(c *Cue) string { return c.UniqueID })
}

func (m *MockServer) findCueByNumber(wsID, num string) *Cue {
	return findCueInList(m.CueLists[wsID], num, func(c *Cue) string { return c.Number })
}

func findCueInList(cues []Cue, val string, key func(*Cue) string) *Cue {
	for i := range cues {
		if key(&cues[i]) == val {
			return &cues[i]
		}
		if found := findCueInList(cues[i].Cues, val, key); found != nil {
			return found
		}
	}
	return nil
}

func (m *MockServer) getCueProperty(cue *Cue, prop string) any {
	switch prop {
	case "uniqueID":
		return cue.UniqueID
	case "number":
		return cue.Number
	case "name":
		return cue.Name
	case "type":
		return cue.Type
	case "colorName":
		return cue.ColorName
	case "flagged":
		return cue.Flagged
	case "armed":
		return cue.Armed
	case "listName":
		return cue.ListName
	default:
		return nil
	}
}

func (m *MockServer) setCueProperty(cue *Cue, prop string, val any) {
	str, _ := val.(string)
	switch prop {
	case "name":
		cue.Name = str
	case "number":
		cue.Number = str
	case "colorName":
		cue.ColorName = str
	case "listName":
		cue.ListName = str
	}
}
