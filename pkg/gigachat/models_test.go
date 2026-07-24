package gigachat

import "testing"

func TestIsKnownChatModel(t *testing.T) {
	tests := []struct {
		name  string
		model string
		want  bool
	}{
		{"GigaChat", "GigaChat", true},
		{"GigaChat-Max", "GigaChat-Max", true},
		{"GigaChat-2-Pro", "GigaChat-2-Pro", true},
		{"unknown model", "SomeFutureModel", false},
		{"empty string", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsKnownChatModel(tt.model); got != tt.want {
				t.Errorf("IsKnownChatModel(%q) = %v, want %v", tt.model, got, tt.want)
			}
		})
	}
}

func TestIsKnownEmbeddingModel(t *testing.T) {
	tests := []struct {
		name  string
		model string
		want  bool
	}{
		{"Embeddings", "Embeddings", true},
		{"EmbeddingsGigaR", "EmbeddingsGigaR", true},
		{"unknown model", "SomeFutureEmbeddings", false},
		{"empty string", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsKnownEmbeddingModel(tt.model); got != tt.want {
				t.Errorf("IsKnownEmbeddingModel(%q) = %v, want %v", tt.model, got, tt.want)
			}
		})
	}
}

func TestKnownChatModels_NotEmpty(t *testing.T) {
	if len(KnownChatModels()) == 0 {
		t.Error("expected at least one known chat model")
	}
}

func TestKnownEmbeddingModels_NotEmpty(t *testing.T) {
	if len(KnownEmbeddingModels()) == 0 {
		t.Error("expected at least one known embedding model")
	}
}

func TestClient_EmbeddingModel_DefaultsWhenEmpty(t *testing.T) {
	c := NewClient(Config{})
	if got := c.embeddingModel(); got != defaultEmbeddingModel {
		t.Errorf("expected default embedding model %q, got %q", defaultEmbeddingModel, got)
	}
}

func TestClient_EmbeddingModel_UsesConfigured(t *testing.T) {
	c := NewClient(Config{EmbeddingModel: string(ModelEmbeddingsGigaR)})
	if got := c.embeddingModel(); got != string(ModelEmbeddingsGigaR) {
		t.Errorf("expected configured embedding model %q, got %q", ModelEmbeddingsGigaR, got)
	}
}
