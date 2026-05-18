package provider

// Ollama runs as a local daemon (`brew services start ollama`) on :11434
// and exposes both its native API and an OpenAI-compatible surface at
// /v1/chat/completions. The default model below is Gemma 4 E4B, optimized
// for reasoning, agentic workflows, and coding. Pull it with:
// `ollama pull gemma4:e4b`. 64 GB+ users can pin larger models in their config:
//
//	"provider_endpoints": { "ollama": { "model": "gemma4:26b" } }
//
// or env: WUPHF_OLLAMA_MODEL=gemma4:26b.
const (
	defaultOllamaBaseURL = "http://127.0.0.1:11434/v1"
	defaultOllamaModel   = "gemma4:e4b"
)

func init() {
	Register(&Entry{
		Kind:     KindOllama,
		StreamFn: NewOpenAICompatStreamFn(KindOllama, defaultOllamaBaseURL, defaultOllamaModel),
		Capabilities: Capabilities{
			PaneEligible:    false,
			SupportsOneShot: false,
		},
	})
}
