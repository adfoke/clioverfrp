package protocol

import "encoding/json"

type Message struct {
	Type       string     `json:"type"`
	Token      string     `json:"token,omitempty"`
	Cmd        string     `json:"cmd,omitempty"`
	Path       string     `json:"path,omitempty"`
	Local      string     `json:"local,omitempty"`
	Remote     string     `json:"remote,omitempty"`
	TempSuffix string     `json:"temp_suffix,omitempty"`
	Data       string     `json:"data,omitempty"`
	Stream     string     `json:"stream,omitempty"`
	SHA256     string     `json:"sha256,omitempty"`
	Error      string     `json:"error,omitempty"`
	Success    bool       `json:"success,omitempty"`
	Force      bool       `json:"force,omitempty"`
	Skip       bool       `json:"skip_existing,omitempty"`
	EOF        bool       `json:"eof,omitempty"`
	Offset     int64      `json:"offset,omitempty"`
	Size       int64      `json:"size,omitempty"`
	Written    int64      `json:"written,omitempty"`
	ChunkSize  int        `json:"chunk_size,omitempty"`
	DurationMS int64      `json:"duration_ms,omitempty"`
	ExitCode   int        `json:"exit_code,omitempty"`
	Entries    []DirEntry `json:"entries,omitempty"`
}

type DirEntry struct {
	Name    string `json:"name"`
	Path    string `json:"path"`
	IsDir   bool   `json:"is_dir"`
	Size    int64  `json:"size"`
	Mode    string `json:"mode"`
	ModTime string `json:"mod_time"`
}

type Result struct {
	Success    bool   `json:"success"`
	Type       string `json:"type"`
	Output     any    `json:"output,omitempty"`
	Error      string `json:"error,omitempty"`
	DurationMS int64  `json:"duration_ms"`
	ExitCode   int    `json:"exit_code"`
}

type TaskLine struct {
	Type   string `json:"type"`
	Agent  string `json:"agent,omitempty"`
	Cmd    string `json:"cmd,omitempty"`
	Path   string `json:"path,omitempty"`
	Local  string `json:"local,omitempty"`
	Remote string `json:"remote,omitempty"`
}

type TaskReport struct {
	Success bool     `json:"success"`
	Count   int      `json:"count"`
	Results []Result `json:"results"`
}

func CloneResult(in Result) Result {
	raw, _ := json.Marshal(in)
	var out Result
	_ = json.Unmarshal(raw, &out)
	return out
}
