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

	embed "github.com/Clinet/discordgo-embed"
	"github.com/bwmarrin/discordgo"
)

// Variables used for command line parameters
var (
	Token string
)

const EmbedPrimaryColor = 0x00CC00
const EmbedErrorColor = 0xCC0000

type channelInfo struct {
	Name string `json:"name"`
	ID   string `json:"id"`
}

type SnoopyConfig struct {
	NotificationChannel  string        `json:"notificationChannel"`
	WatchedVoiceChannels []channelInfo `json:"WatchedVoiceChannels"`
}

var config map[string]*SnoopyConfig

func init() {

	flag.StringVar(&Token, "t", "", "Bot Token")
	flag.Parse()

	cfg, err := readConfigFromFile()
	if err != nil {
		fmt.Println("unable to read config file, using default/empty config")
		fmt.Println(err)
		config = map[string]*SnoopyConfig{}
	} else {
		config = cfg
	}
}

func writeConfigToFile(config map[string]*SnoopyConfig) {
	fmt.Println("Writing snoopyConfig.json")
	jsonData, err := json.Marshal(config)
	if err != nil {
		log.Fatal(err)
	}

	ioutil.WriteFile("snoopyConfig.json", jsonData, os.ModePerm)
}

func readConfigFromFile() (map[string]*SnoopyConfig, error) {
	fmt.Println("Reading snoopyConfig.json")
	rawData, err := ioutil.ReadFile("snoopyConfig.json")
	if err != nil {
		return nil, err
	}
	config := map[string]*SnoopyConfig{}
	err = json.Unmarshal(rawData, &config)

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

	if userUpdated || config[guild] == nil || config[guild].NotificationChannel == "" {
		return
	}

	channelName := ""
	for i := 0; i < len(config[guild].WatchedVoiceChannels); i++ {
		if config[guild].WatchedVoiceChannels[i].ID == newChannelId {
			userJoined = true
			channelName = config[guild].WatchedVoiceChannels[i].Name
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
		var msg string
		if memberCount == 1 {
			msg = fmt.Sprintf("<@%v> started a voice chat in <#%v>", v.Member.User.ID, newChannelId)
		} else {
			msg = fmt.Sprintf("<@%v> joined a voice chat in <#%v> with %v members", v.Member.User.ID, newChannelId, memberCount)
		}
		emb := embed.NewEmbed().SetDescription(msg).SetColor(EmbedPrimaryColor)
		s.ChannelMessageSendEmbeds(config[v.GuildID].NotificationChannel, []*discordgo.MessageEmbed{emb.MessageEmbed})
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
		s.ChannelMessageSendEmbeds(m.ChannelID, []*discordgo.MessageEmbed{embed.NewGenericEmbedAdvanced("Snoopy Help",
			"`snoopy setchannel`\n- sets the text channel where notifications will be sent\n"+
				"`snoopy watchchannel <channel name>`\n- adds a voice channel to the list of watched channels\n"+
				"`snoopy unwatchchannel <channel name>`\n- removes a voice channel to the list of watched channels\n", EmbedPrimaryColor)})
		return
	}

	if m.Content == "snoopy setchannel" {
		s.ChannelMessageSendEmbeds(m.ChannelID, []*discordgo.MessageEmbed{embed.NewGenericEmbedAdvanced("Snoopy",
			"Using this channel for voice chat notifications!", EmbedPrimaryColor)})
		if config[m.GuildID] == nil {
			config[m.GuildID] = &SnoopyConfig{NotificationChannel: m.ChannelID}
		} else {
			config[m.GuildID].NotificationChannel = m.ChannelID
		}
		writeConfigToFile(config)
		return
	}

	if strings.HasPrefix(m.Content, "snoopy watchchannel") {
		if len(m.Content) <= len("snoopy watchchannel ") {
			s.ChannelMessageSendEmbeds(m.ChannelID, []*discordgo.MessageEmbed{embed.NewGenericEmbedAdvanced("Snoopy",
				fmt.Sprintf("Unable to set watch channel, please specify a voice channel name"), EmbedErrorColor)})
			return
		}

		channelName := m.Content[len("snoopy watchchannel "):]

		channels, err := s.GuildChannels(m.GuildID)
		if err != nil {
			fmt.Println(err)
			return
		}
		channelID := getChannelIdFromName(channels, channelName)
		if channelID == "" {
			s.ChannelMessageSendEmbeds(m.ChannelID, []*discordgo.MessageEmbed{embed.NewGenericEmbedAdvanced("Snoopy",
				fmt.Sprintf("Requested channel (%v) not found", channelName), EmbedErrorColor)})
			return
		}

		fmt.Println(fmt.Sprintf("watchchannel: %v (%v)", channelName, channelID))
		s.ChannelMessageSendEmbeds(m.ChannelID, []*discordgo.MessageEmbed{embed.NewGenericEmbedAdvanced("Snoopy",
			fmt.Sprintf("Voice channel <#%v> added to watch list", channelID), EmbedPrimaryColor)})

		if config[m.GuildID] == nil {
			config[m.GuildID] = &SnoopyConfig{WatchedVoiceChannels: []channelInfo{{channelName, channelID}}}
		} else {
			exists := false
			for _, watched := range config[m.GuildID].WatchedVoiceChannels {
				if watched.ID == channelID {
					exists = true
				}
			}
			if !exists {
				config[m.GuildID].WatchedVoiceChannels = append(config[m.GuildID].WatchedVoiceChannels, channelInfo{channelName, channelID})
			} else {
				fmt.Println("channel already in watched list, skipping")
			}
		}

		writeConfigToFile(config)
	}

	if strings.HasPrefix(m.Content, "snoopy unwatchchannel") {
		if len(m.Content) <= len("snoopy unwatchchannel ") {
			s.ChannelMessageSendEmbeds(m.ChannelID, []*discordgo.MessageEmbed{embed.NewGenericEmbedAdvanced("Snoopy",
				fmt.Sprintf("Unable to unset watch channel, please specify a voice channel name"), EmbedErrorColor)})
			return
		}

		channelName := m.Content[len("snoopy unwatchchannel "):]

		channels, err := s.GuildChannels(m.GuildID)
		if err != nil {
			fmt.Println(err)
			return
		}
		channelID := getChannelIdFromName(channels, channelName)
		if channelID == "" {
			s.ChannelMessageSendEmbeds(m.ChannelID, []*discordgo.MessageEmbed{embed.NewGenericEmbedAdvanced("Snoopy",
				fmt.Sprintf("Requested channel (%v) not found", channelName), EmbedErrorColor)})
			return
		}

		fmt.Println(fmt.Sprintf("unwatchchannel: %v (%v)", channelName, channelID))
		s.ChannelMessageSendEmbeds(m.ChannelID, []*discordgo.MessageEmbed{embed.NewGenericEmbedAdvanced("Snoopy",
			fmt.Sprintf("Voice channel <#%v> removed from watch list", channelID), EmbedPrimaryColor)})

		if config[m.GuildID] != nil {
			var foundIndex = -1
			for idx, watched := range config[m.GuildID].WatchedVoiceChannels {
				if watched.ID == channelID {
					foundIndex = idx
				}
			}
			if foundIndex >= 0 {
				config[m.GuildID].WatchedVoiceChannels = removeIndex(config[m.GuildID].WatchedVoiceChannels, foundIndex)
			} else {
				fmt.Println("unable to remove channel that does not exist in our list, skipping")
			}
		}

		writeConfigToFile(config)
	}
}

func removeIndex(s []channelInfo, index int) []channelInfo {
	return append(s[:index], s[index+1:]...)
}
