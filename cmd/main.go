package main

import (
	"fmt"
	"os"

	openai "github.com/StevenDStanton/openaigo"
	"github.com/StevenDStanton/openaigo/chat"
	"github.com/joho/godotenv"
)

func main() {
	//server.StartServer()
	err := godotenv.Load()
	if err != nil {
		panic("We really need that .env file...")
	}
	openAiApiKey := os.Getenv("OPENAI_APIKEY")
	client := openai.NewOpenAIClient(openAiApiKey)
	message := chat.Message{
		Content: "Tell me about golang",
		Role:    chat.User,
	}

	messages := []chat.Message{message}
	response, err := client.Chat.NewChatRequest(chat.GPT4o, messages, chat.Text)

	if err != nil {
		fmt.Println(fmt.Errorf("error: %w", err))
		return
	}

	fmt.Printf("%v", response)

}
