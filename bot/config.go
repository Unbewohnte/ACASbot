package bot

type Config struct {
	ApiToken         string
	OrganizationName string
	OllamaModel      string
	MaxContentSize   uint
	Debug            bool
	FullAnalysis     bool
}

func NewConfig(
	apiToken string,
	organizationName string,
	ollamaModel string,
	maxContentSize uint,
	debug bool,
	fullAnalysis bool,
) Config {
	return Config{
		ApiToken:         apiToken,
		OrganizationName: organizationName,
		OllamaModel:      ollamaModel,
		MaxContentSize:   maxContentSize,
		Debug:            debug,
		FullAnalysis:     fullAnalysis,
	}
}
