package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"goia/structs"
	"io"
	"log"
	"net/http"
	"os"
	"time"
	"strings"

	"github.com/go-resty/resty/v2"
	"github.com/joho/godotenv"
	"github.com/patrickmn/go-cache"
)

const OPENAI_API_URL = "https://api.groq.com/openai/v1/chat/completions"

// Cria um cache com tempo de expiração padrão de 1 semana e purga itens não utilizados a cada 10 minutos
var c = cache.New(7*24*time.Hour, 10*time.Minute)

func generateHash(prompt string) string {
	hash := sha256.Sum256([]byte(prompt))
	return hex.EncodeToString(hash[:])
}

func main() {
	// Carrega variáveis de ambiente do arquivo .env
	err := godotenv.Load()
	if err != nil {
		log.Fatal("Erro ao carregar o arquivo .env")
	}

	// Obtém a chave API do ambiente
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		log.Fatal("OPENAI_API_KEY não está definida no arquivo .env")
	}

	// Cria um cliente Resty
	client := resty.New()

	// Define o handler para a rota /generate
	http.HandleFunc("/generate", func(w http.ResponseWriter, r *http.Request) {
		// Verifica se o método é POST
		if r.Method != "POST" {
			http.Error(w, "Método não permitido", http.StatusMethodNotAllowed)
			return
		}

		// Lê o corpo da requisição
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "Corpo da requisição inválido", http.StatusBadRequest)
			return
		}

		// Faz o parse do JSON de entrada
		var input structs.Input

		err = json.Unmarshal(body, &input)
		if err != nil {
			http.Error(w, "Formato JSON inválido", http.StatusBadRequest)
			return
		}

		prompt := input.Prompt
		system := input.System

		// Gera o hash para o prompt
		promptHash := generateHash(system + prompt)

		// Verifica se a resposta está em cache
		if cachedResponse, found := c.Get(promptHash); found {
			responseContent := cachedResponse.(string)

			// Retorna a resposta em cache
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]string{
				"response": responseContent,
			})
			return
		}

		// Prepara a requisição para a API OpenAI
		request := structs.OpenAIRequest{
			Model: "gemma-7b-it", // Você pode mudar para "gpt-4" se tiver acesso
			Messages: []structs.Message{
				{Role: "system", Content: system},
				{Role: "user", Content: prompt},
			},
		}

		// Faz a requisição para a API da OpenAI
		var response structs.OpenAIResponse
		_, err = client.R().
			SetHeader("Authorization", "Bearer "+apiKey).
			SetHeader("Content-Type", "application/json").
			SetBody(request).
			SetResult(&response).
			Post(OPENAI_API_URL)

		if err != nil {
			http.Error(w, fmt.Sprintf("Erro ao fazer a requisição: %v", err), http.StatusInternalServerError)
			return
		}

		// Verifica se recebeu uma resposta
		if len(response.Choices) > 0 {
			responseContent := response.Choices[0].Message.Content
			responseContent = strings.ReplaceAll(responseContent, "*", "")

			// Armazena a resposta em cache por 1 semana
			c.Set(promptHash, responseContent, cache.DefaultExpiration)

			// Retorna a resposta
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]string{
				"response": responseContent,
			})
		} else {
			http.Error(w, "Nenhuma resposta recebida", http.StatusInternalServerError)
		}
	})

	// Inicia o servidor na porta 8080
	fmt.Println("Servidor rodando na porta 8080...")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
