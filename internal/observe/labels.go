package observe

// The assistant names the playmat gives the clients it knows
// (mcpserver.AssistantName); any other client's self-chosen name is
// "other" in spans and counters, so it cannot multiply series.
var knownAssistants = map[string]bool{"Claude": true, "ChatGPT": true, "Codex": true, "Gemini": true}

// collapse is name if it is a known assistant, else "other".
func collapse(name string) string {
	if knownAssistants[name] {
		return name
	}
	return "other"
}

// provider is the sign-in provider label: as the record names it, or
// "google" when it names none (sign-ins recorded before providers were).
// The playmat's own code names the provider, never a client, so the label
// stays a closed set (google today, password once that sign-in ships).
func provider(p string) string {
	if p == "" {
		return "google"
	}
	return p
}

// oneOf is v if it is one of allowed, else "other".
func oneOf(v string, allowed ...string) string {
	for _, a := range allowed {
		if v == a {
			return v
		}
	}
	return "other"
}
