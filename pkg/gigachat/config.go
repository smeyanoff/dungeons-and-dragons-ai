package gigachat

// Config содержит параметры для GigaChat API.
type Config struct {
	AuthBaseURL      string // https://ngw.devices.sberbank.ru:9443
	APIBaseURL       string // https://gigachat.devices.sberbank.ru/api/v1
	ClientID         string
	ClientSecret     string
	Scope            string // например "GIGACHAT_API_PERS"
	SkipTLSVerify    bool   // Пропустить проверку TLS сертификата (только для тестов)
	ConcurrencyLimit int    // Максимальное количество одновременных запросов (по умолчанию 5)
	RPSLimit         float64 // Максимальное количество запросов в секунду (по умолчанию 10.0)
	RateBurst        int     // Burst limit для rate limiter (по умолчанию 5)
}
