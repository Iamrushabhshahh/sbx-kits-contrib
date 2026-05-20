package spec

// MCPClientSpec declares how a kit configures its agent to talk to the
// sandbox's aggregate MCP gateway. The consuming engine reads this spec
// when a runtime starts the gateway and applies the declared files and
// commands inside the container.
//
// The spec is intentionally declarative: paths, contents, and command
// argv elements are Go text/template strings rendered against a context
// supplied by the engine (gateway URL, sentinel token, runtime/workspace
// dirs). The spec library only defines the data shape; template parsing
// and rendering live in the consumer.
type MCPClientSpec struct {
	// Files to write inside the container after the gateway is up. Each
	// file's Path and Content are rendered as Go templates by the
	// consumer.
	Files []MCPClientFile `json:"files,omitempty" yaml:"files,omitempty"`

	// Commands to exec inside the container after the files are written.
	// Use sparingly — most agents need only a config file write.
	Commands []MCPClientCommand `json:"commands,omitempty" yaml:"commands,omitempty"`
}

// MCPClientFile describes one file the kit needs the engine to write
// inside the container.
type MCPClientFile struct {
	// Path is the absolute path inside the container where the file
	// should be written. Rendered as a Go template by the consumer.
	Path string `json:"path" yaml:"path"`

	// Content is the file body. Rendered as a Go template by the consumer.
	Content string `json:"content" yaml:"content"`

	// Mode is the file mode (octal). Zero falls back to 0o644 at apply
	// time.
	Mode int `json:"mode,omitempty" yaml:"mode,omitempty"`
}

// MCPClientCommand describes one command the kit wants the engine to
// exec inside the container after the files are written. Run via exec,
// not via a shell, so each argv element is rendered independently and
// no shell interpolation occurs.
type MCPClientCommand struct {
	// Argv is the exec argv. Each element is rendered as a Go template
	// by the consumer.
	Argv []string `json:"argv" yaml:"argv"`
}
