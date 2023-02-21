package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/bwmarrin/discordgo"
)

// Variables used for command line parameters
var (
	Token string
)

type channelInfo struct {
	Name string `json:"name"`
	ID   string `json:"id"`
}

type SnoopyConfig struct {
	NotificationChannel  string        `json:"notificationChannel"`
	WatchedVoiceChannels []channelInfo `json:"WatchedVoiceChannels"`
}

var config *SnoopyConfig

func init() {

	flag.StringVar(&Token, "t", "", "Bot Token")
	flag.Parse()

	cfg, err := readConfigFromFile()
	if err != nil {
		fmt.Println("unable to read config file, using default/empty config")
		config = &SnoopyConfig{}
	} else {
		config = cfg
	}
}

func writeConfigToFile(config *SnoopyConfig) {
	fmt.Println("Writing snoopyConfig.json")
	jsonData, err := json.Marshal(config)
	if err != nil {
		log.Fatal(err)
	}

	ioutil.WriteFile("snoopyConfig.json", jsonData, os.ModePerm)
}

func readConfigFromFile() (*SnoopyConfig, error) {
	fmt.Println("Reading snoopyConfig.json")
	rawData, err := ioutil.ReadFile("snoopyConfig.json")
	if err != nil {
		return nil, err
	}
	config := &SnoopyConfig{}
	err = json.Unmarshal(rawData, config)

	return config, err
}

func main() {

	// Create a new Discord session using the provided bot token.
	dg, err := discordgo.New("Bot " + Token)
	if err != nil {
		fmt.Println("error creating Discord session,", err)
		return
	}

	// Register the messageCreate func as a callback for MessageCreate events.
	dg.AddHandler(messageCreate)
	dg.AddHandler(voiceStateUpdate)

	// In this example, we only care about receiving message events.
	//dg.Identify.Intents = discordgo.IntentsGuildMessages | discordgo.IntentGuildVoiceStates | discordgo.IntentGuildMembers
	dg.Identify.Intents = discordgo.IntentsAll

	// Open a websocket connection to Discord and begin listening.
	err = dg.Open()
	if err != nil {
		fmt.Println("error opening connection,", err)
		return
	}

	// Wait here until CTRL-C or other term signal is received.
	fmt.Println("Bot is now running.  Press CTRL-C to exit.")
	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM, os.Interrupt)
	<-sc

	// Cleanly close down the Discord session.
	dg.Close()
}

func voiceStateUpdate(s *discordgo.Session, v *discordgo.VoiceStateUpdate) {

	old := v.BeforeUpdate
	new := v
	fmt.Println(fmt.Sprintf("voice update"))

	if old == nil {
		old = &discordgo.VoiceState{}
	}

	userUpdated := new.ChannelID == old.ChannelID
	userJoined := false
	newChannelId := new.ChannelID
	//oldChannelId := old.ChannelID
	guild := new.GuildID
	if guild == "" {
		guild = old.GuildID
	}
	userName := new.Member.Nick
	if userName == "" {
		userName = old.Member.Nick
	}

	if userUpdated || config.NotificationChannel == "" {
		return
	}

	channelName := ""
	for i := 0; i < len(config.WatchedVoiceChannels); i++ {
		if config.WatchedVoiceChannels[i].ID == newChannelId {
			userJoined = true
			channelName = config.WatchedVoiceChannels[i].Name
		}
	}

	if channelName == "" {
		return
	}

	// lets try to figure out how many other people are in this voice channel - discord permissions are wonky...
	memberCount := 0
	g, err := s.State.Guild(v.GuildID)
	if err == nil {
		for _, vs := range g.VoiceStates {
			if vs.ChannelID == newChannelId {
				memberCount++
			}
		}
	}

	fmt.Println(fmt.Sprintf("User %v joined channel %v with %v members", v.Member.Nick, channelName, memberCount))

	if userJoined {
		s.ChannelMessageSend(config.NotificationChannel, fmt.Sprintf("<@%v> joined <#%v> with %v members", v.Member.User.ID, newChannelId, memberCount))
	}

}

func getChannelIdFromName(channels []*discordgo.Channel, name string) string {
	for i := 0; i < len(channels); i++ {
		if channels[i].Name == name {
			return channels[i].ID
		}
	}

	return ""
}

// This function will be called (due to AddHandler above) every time a new
// message is created on any channel that the authenticated bot has access to.
func messageCreate(s *discordgo.Session, m *discordgo.MessageCreate) {

	// Ignore all messages created by the bot itself
	// This isn't required in this specific example but it's a good practice.
	if m.Author.ID == s.State.User.ID {
		return
	}

	if m.Content == "snoopy help" {
		s.ChannelMessageSend(m.ChannelID,
			`snoopy setchannel
    sets the text channel where notifications will be sent
snoopy watchchannel <channel name>
    adds a voice channel to the list of watched voice channels`)
		return
	}

	if m.Content == "snoopy setchannel" {
		s.ChannelMessageSend(m.ChannelID, "Using this channel for voice chat notifications!")
		config.NotificationChannel = m.ChannelID
		writeConfigToFile(config)
		return
	}

	if strings.HasPrefix(m.Content, "snoopy watchchannel ") {
		channelName := m.Content[len("snoopy watchchannel "):]
		if len(channelName) == 0 {
			s.ChannelMessageSend(m.ChannelID, "Unable to set watch channel, please specify a voice channel name")
			return
		}

		channels, err := s.GuildChannels(m.GuildID)
		if err != nil {
			fmt.Println(err)
			return
		}
		channelID := getChannelIdFromName(channels, channelName)
		if channelID == "" {
			s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("Requested channel (%v) not found", channelName))
			return
		}

		fmt.Println(fmt.Sprintf("watchchannel: %v (%v)", channelName, channelID))
		config.WatchedVoiceChannels = append(config.WatchedVoiceChannels, channelInfo{channelName, channelID})
		writeConfigToFile(config)
	}

}
