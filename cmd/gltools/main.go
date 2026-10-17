package main

// Result is a single output of a tool.
// Name is the short description shown next to it, Value is the text that gets copied.
// Items, used by Pick tools, are the entries of a further list opened from this one;
// then Value is a line shown above that list.
type Result struct {
	Name  string
	Value string
	Items []Result
}

// Tool is a menu entry.
// If Prompt is set, the user is asked for a text first, which is passed to Run.
// If Menu is set, the entry opens a submenu with those tools.
// A single result is copied right away; with CopyFirst the first one is copied even if there are more.
// With CopyAll the results are a report: single rows cannot be copied, "c" copies all of them.
// Info, if set, gives lines shown above the prompt (e.g. what already exists).
// Stream can be used instead of Run by tools that take time: it calls show with
// the results found so far, which appear on screen while the tool is still working.
// With Again the prompt stays open after each run: the outcome is shown above it, nothing
// is copied, and enter on an empty prompt (or esc) goes back to the menu.
// With View (together with Again) the first result is shown full screen to scroll through;
// esc goes back to the prompt. PlainView leaves out the line numbers.
// With Pick (together with Again) the results are a list to choose from: enter opens the
// Items of the chosen one as a further list, or shows its Value in the viewer when it has none.
// Esc goes back one list at a time, and from the first one to the prompt.
// With ViewCopy, "c" in the viewer copies the text shown and goes back like esc.
type Tool struct {
	Name      string
	Prompt    string
	Run       func(input string) ([]Result, error)
	Stream    func(input string, show func([]Result)) ([]Result, error)
	Menu      []Tool
	CopyFirst bool
	CopyAll   bool
	Info      func() []string
	Again     bool
	View      bool
	PlainView bool
	Pick      bool
	ViewCopy  bool
}

// To add a new tool, just add an entry to this list or to one of its submenus.
var tools = []Tool{
	{Name: "Security", Menu: []Tool{
		{Name: "Random strings", Run: func(string) ([]Result, error) { return generatePasswords(), nil }},
		{Name: "Generate hashes", Prompt: "Type or paste the text", Run: func(s string) ([]Result, error) { return generateHashes(s), nil }},
		{Name: "SSH key pair (Ed25519)", Prompt: "Name for the key, saved in ~/.ssh as id_ed25519_<name> (e.g. github)", Run: generateSSHKey, CopyFirst: true, Info: existingSSHKeys},
		{Name: "Decode JWT", Prompt: "Paste the token", Run: decodeJWT},
	}},
	{Name: "Network", Menu: []Tool{
		{Name: "My IP address", Run: func(string) ([]Result, error) { return myIP() }, CopyAll: true},
		{Name: "Ping", Prompt: "Type or paste the address (e.g. google.com or 192.168.1.1)", Stream: ping, CopyAll: true},
		{Name: "Check a website", Prompt: "Type or paste the address (e.g. google.com)", Run: checkWebsite, CopyAll: true},
		{Name: "DNS lookup", Prompt: "Type or paste the domain (e.g. google.com), or an IP to find its name", Run: dnsLookup, CopyAll: true},
		{Name: "IP info", Prompt: "IP address or domain (leave empty for your public IP)", Run: ipInfo, CopyAll: true},
		{Name: "Port check", Prompt: "Address and port (e.g. example.com:22), or only the address to check the common ports", Run: portCheck, CopyAll: true},
	}},
	{Name: "JSON/XML", Menu: []Tool{
		{Name: "Validate JSON/XML", Prompt: "Paste the JSON or XML text", Run: validateJSONorXML, Again: true},
		{Name: "View JSON/XML", Prompt: "Paste the JSON or XML text", Run: formatJSONorXML, Again: true, View: true},
		{Name: "Structure of JSON/XML", Prompt: "Paste the JSON or XML text", Run: structureOf, Again: true, View: true, PlainView: true, ViewCopy: true},
		{Name: "Skeleton of JSON/XML", Prompt: "Paste the JSON or XML text", Run: skeletonOf, Again: true, View: true, ViewCopy: true},
		{Name: "OpenAPI JSON examples", Prompt: "Paste the OpenAPI specification (YAML or JSON)", Run: openAPIExamples, Again: true, Pick: true, ViewCopy: true},
		{Name: "XSD XML example", Prompt: "Paste the XSD schema", Run: xsdExamples, Again: true, Pick: true, ViewCopy: true},
	}},
}

func main() {
	runWindow()
}
