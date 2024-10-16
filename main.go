package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"

	_ "github.com/mattn/go-sqlite3" // SQLite driver for crypto helper
	"github.com/pelletier/go-toml"  // Import go-toml for config parsing
	mautrix "maunium.net/go/mautrix"
	"maunium.net/go/mautrix/crypto/cryptohelper"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"
)

// Config struct to load from TOML
type Config struct {
	Bot struct {
		Homeserver   string `toml:"homeserver"`
		Username     string `toml:"username"`
		Password     string `toml:"password"`
		UserID       string `toml:"user_id"`
		CryptoDBPath string `toml:"crypto_db_path"`
	}
	LLM struct {
		Model         string `toml:"model"`
		DefaultPrompt string `toml:"default_prompt"`
	}
}

// Structure to manage user conversation history and system prompts
type UserState struct {
	conversation []OllamaMessage
	systemPrompt string
}

// A map to store conversation history per user
var userConversations = make(map[id.UserID]*UserState)
var conversationLock sync.Mutex

// Config variable to hold the loaded configuration
var config Config

func main() {
	// Load the configuration from config.toml
	err := loadConfig("config.toml")
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	// Create the Matrix client
	client, err := mautrix.NewClient(config.Bot.Homeserver, "", "")
	if err != nil {
		log.Fatalf("Failed to create client: %v", err)
	}

	// Log in to Matrix using the context
	ctx := context.TODO()
	loginResponse, err := client.Login(ctx, &mautrix.ReqLogin{
		Type: mautrix.AuthTypePassword,
		Identifier: mautrix.UserIdentifier{
			Type: mautrix.IdentifierTypeUser,
			User: config.Bot.Username,
		},
		Password: config.Bot.Password,
	})
	if err != nil {
		log.Fatalf("Failed to login: %v", err)
	}

	// Set the client UserID and AccessToken from the login response
	client.UserID = loginResponse.UserID
	client.AccessToken = loginResponse.AccessToken

	// Set up the crypto helper for encryption handling
	cryptoHelper, err := cryptohelper.NewCryptoHelper(client, []byte("uwu rawr x3"), config.Bot.CryptoDBPath)
	if err != nil {
		log.Fatalf("Failed to set up encryption: %v", err)
	}

	cryptoHelper.LoginAs = &mautrix.ReqLogin{
		Type:       mautrix.AuthTypePassword,
		Identifier: mautrix.UserIdentifier{Type: mautrix.IdentifierTypeUser, User: config.Bot.Username},
		Password:   config.Bot.Password,
	}

	// Initialize crypto helper
	err = cryptoHelper.Init(ctx)
	if err != nil {
		log.Fatalf("Failed to initialize crypto: %v", err)
	} else {
		log.Println("Crypto initialized.")
	}
	client.Crypto = cryptoHelper

	// Check if the database file is being created
	if _, err := os.Stat(config.Bot.CryptoDBPath); os.IsNotExist(err) {
		log.Fatalf("Database file was not created at: %s", config.Bot.CryptoDBPath)
	} else {
		log.Println("Database file exists: ", config.Bot.CryptoDBPath)
	}

	// Set up the syncer to listen for events
	syncer := client.Syncer.(*mautrix.DefaultSyncer)

	// Event handler for messages (make sure this listens for messages properly)
	syncer.OnEventType(event.EventMessage, func(ctx context.Context, ev *event.Event) {
		log.Printf("Received message event from %s in room %s", ev.Sender, ev.RoomID)
		handleMessage(ctx, client, ev)
	})

	// Automatically join rooms when invited
	syncer.OnEventType(event.StateMember, func(ctx context.Context, ev *event.Event) {
		if ev.GetStateKey() == client.UserID.String() && ev.Content.AsMember().Membership == event.MembershipInvite {
			_, err := client.JoinRoomByID(ctx, ev.RoomID)
			if err == nil {
				log.Printf("Joined room %s after invite", ev.RoomID)
			} else {
				log.Printf("Failed to join room %s: %v", ev.RoomID, err)
			}
		}
	})

	// Start syncing
	syncCtx, cancelSync := context.WithCancel(context.Background())
	defer cancelSync()

	go func() {
		if err = client.SyncWithContext(syncCtx); err != nil {
			log.Fatalf("Sync failed: %v", err)
		}
	}()

	fmt.Println("All systems nominal! Running...")

	// Keep the bot running
	select {}
}

// Load the TOML config from file
func loadConfig(filename string) error {
	data, err := toml.LoadFile(filename)
	if err != nil {
		return err
	}
	err = data.Unmarshal(&config)
	if err != nil {
		return fmt.Errorf("failed to unmarshal config: %v", err)
	}
	return nil
}

// Function to handle incoming messages and commands
func handleMessage(ctx context.Context, client *mautrix.Client, ev *event.Event) {
	// Ensure only the allowed user can interact
	if ev.Sender.String() != config.Bot.UserID {
		log.Printf("Message ignored from %s", ev.Sender)
		return
	}

	content := ev.Content.AsMessage()
	if content == nil || content.Body == "" {
		log.Printf("Empty or nil message body received from %s", ev.Sender)
		return
	}

	log.Printf("Processing message from %s: %s", ev.Sender, content.Body)

	// Check if it's a DM room by inspecting the room members
	membersResp, err := client.JoinedMembers(ctx, ev.RoomID)
	if err != nil {
		log.Printf("Failed to get room members: %v", err)
		return
	}

	if len(membersResp.Joined) == 2 {
		// This is a DM room
		log.Println("This is a DM room.")
		processMessage(ctx, client, ev, content.Body)
	} else {
		log.Println("Message is from a group room, ignoring.")
	}
}

// Function to process the actual message
func processMessage(ctx context.Context, client *mautrix.Client, ev *event.Event, body string) {
	conversationLock.Lock()
	state, exists := userConversations[ev.Sender]
	if !exists {
		state = &UserState{
			conversation: []OllamaMessage{},
			systemPrompt: config.LLM.DefaultPrompt,
		}
		userConversations[ev.Sender] = state
	}
	conversationLock.Unlock()

	// Handle commands
	if strings.HasPrefix(body, "/clear") {
		// Clear the conversation
		conversationLock.Lock()
		state.conversation = []OllamaMessage{}
		conversationLock.Unlock()
		client.SendText(ctx, ev.RoomID, "Conversation cleared.")
		return
	} else if strings.HasPrefix(body, "/setprompt") {
		// Set a new system prompt
		newPrompt := strings.TrimSpace(strings.TrimPrefix(body, "/setprompt"))
		conversationLock.Lock()
		state.systemPrompt = newPrompt
		conversationLock.Unlock()
		client.SendText(ctx, ev.RoomID, "System prompt set.")
		return
	} else if strings.HasPrefix(body, "/viewprompt") {
		// View the current system prompt
		conversationLock.Lock()
		prompt := state.systemPrompt
		conversationLock.Unlock()
		client.SendText(ctx, ev.RoomID, "Current system prompt: "+prompt)
		return
	} else if strings.HasPrefix(body, "/viewconversation") {
		// View the conversation history
		conversationLock.Lock()
		conversation := state.conversation
		conversationLock.Unlock()
		conversationText := "Conversation:\n"
		for _, message := range conversation {
			conversationText += fmt.Sprintf("[%s] %s\n", message.Role, message.Content)
		}
		client.SendText(ctx, ev.RoomID, conversationText)
		return
	} else if strings.HasPrefix(body, "/help") {
		// Display help message
		helpMessage := "Commands:\n" +
			"/clear - Clear the conversation history\n" +
			"/setprompt <prompt> - Set a new system prompt\n" +
			"/viewprompt - View the current system prompt\n" +
			"/viewconversation - View the conversation history\n" +
			"/help - Display this help message"
		client.SendText(ctx, ev.RoomID, helpMessage)
		return
	}

	// Add user message to conversation history
	conversationLock.Lock()
	state.conversation = append(state.conversation, OllamaMessage{Role: "user", Content: body})
	conversationLock.Unlock()

	// Query Ollama with the full conversation
	response, err := queryOllama(body, state.conversation, state.systemPrompt)
	if err != nil {
		log.Printf("Failed to query Ollama: %v", err)
		client.SendText(ctx, ev.RoomID, "Sorry, something went wrong.")
		return
	}

	// Add the LLM response to the conversation history
	conversationLock.Lock()
	state.conversation = append(state.conversation, OllamaMessage{Role: "you", Content: response})
	conversationLock.Unlock()

	// Send the LLM's response to the Matrix room
	_, err = client.SendText(ctx, ev.RoomID, response)
	if err != nil {
		log.Printf("Failed to send message: %v", err)
	}
}
