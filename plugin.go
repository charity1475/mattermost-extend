package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"mattermost-extend/configuration"
	"mattermost-extend/configuration/language"
	"mattermost-extend/helper"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"github.com/mattermost/mattermost-server/model"
	"github.com/mattermost/mattermost-server/plugin"
	"github.com/pkg/errors"
)

type MMPlugin struct {
	plugin.MattermostPlugin
}

func main() {
	plugin.ClientMain(&MMPlugin{})
}

func (p *MMPlugin) MessageHasBeenPosted(c *plugin.Context, post *model.Post) {

	//Regular expression used for the replacement logic of incoming and outgoing webhooks
	r, _ := regexp.Compile("^\\S+")
	triggerWord := r.FindString(post.Message)

	if helper.Contains(configuration.ChatWithMeTriggerWords, triggerWord) {
		SendPostToChatWithMeExtension(post, triggerWord, p)
	}

	//Regular expression user for special commands like: open, create, edit, list that
	r, _ = regexp.Compile("^#(\\w+) (\\w+)(?: (\\d+))?$")
	matches := r.FindStringSubmatch(strings.TrimSpace(post.Message))

	if len(matches) > 0 {

		if action, ok := language.Command[matches[1]]; ok {

			module := matches[2]

			broadcast := &model.WebsocketBroadcast{UserId: post.UserId}

			payloadData := map[string]interface{}{
				"action": action,
				"module": module,
			}

			if matches[3] != "" {
				payloadData["id"] = matches[3]
			}

			p.API.PublishWebSocketEvent("corebos", payloadData, broadcast)
		}
	}

}

func (p *MMPlugin) OnActivate() error {

	teams, err := p.API.GetTeams()
	if err != nil {
		return err
	}

	if len(teams) == 0 {
		return errors.New("there are no existing teams")
	}

	team := teams[0]
	channel, _ := p.API.GetChannelByNameForTeamName(team.Name, "chatwithme", false)

	if channel == nil {

		channel, err = p.API.CreateChannel(&model.Channel{
			TeamId:      team.Id,
			Type:        model.CHANNEL_OPEN,
			DisplayName: "Chat With Me",
			Name:        "chatwithme",
			Header:      "The channel used by the mattermost-extend plugin.",
			Purpose:     "The channel was created by the mattermost-extend plugin to extend the server functionality.",
		})

		if err != nil {
			return err
		}

	}

	return nil
}

func SendPostToChatWithMeExtension(post *model.Post, triggerWord string, p *MMPlugin) error {

	cnl, _ := p.API.GetChannel(post.ChannelId)

	formData := url.Values{
		"text":         {post.Message},
		"token":        {configuration.ChatWithMeToken},
		"trigger_word": {triggerWord},
		"user_id":      {post.UserId},
		"channel_id":   {post.ChannelId},
		"chnl_name":    {cnl.Name},
		"chnl_dname":   {cnl.DisplayName},
	}

	newPost := &model.Post{
		UserId:    post.UserId,
		ChannelId: post.ChannelId,
		Type:      model.POST_SLACK_ATTACHMENT,
	}
	resp, err := http.PostForm(configuration.ChatWithMeExtensionUrl, formData)
	defer resp.Body.Close()

	if err != nil {
		return err
	}

	incomingWebhookPayload, decodeError := model.IncomingWebhookRequestFromJson(resp.Body)
	if decodeError != nil {
		return decodeError
	}

	if len(incomingWebhookPayload.Text) == 0 && incomingWebhookPayload.Attachments == nil {
		return errors.New("Wrong response format")
	}

	if incomingWebhookPayload.Props != nil {
		newPost.Props = incomingWebhookPayload.Props
	}
	newPost.Message = incomingWebhookPayload.Text
	newPost.AddProp("attachments", incomingWebhookPayload.Attachments)

	p.API.SendEphemeralPost(newPost.UserId, newPost)
	return nil
}

func (p *MMPlugin) ServeHTTP(c *plugin.Context, w http.ResponseWriter, r *http.Request) {

	switch r.URL.Path {
	case "/syncuser":
		p.syncUserWithcoreBOS(c, w, r)
	case "/hello":
		p.handleHello(w, r)
	case "/postmessage":
		p.postMessage(c, w, r)
	default:
		http.NotFound(w, r)
	}

}

func (p *MMPlugin) syncUserWithcoreBOS(c *plugin.Context, w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	rawBody, err := ioutil.ReadAll(r.Body)
	if err != nil {
		fmt.Fprintln(w, "Errror Geting body")
		return
	}

	userRequest := configuration.User{}
	err = json.Unmarshal(rawBody, &userRequest)
	if err != nil {
		fmt.Fprintln(w, "Errror Decoding Json user")
		return
	}

	userCreate := userRequest.GetMMUser()
	userExist, appError := p.API.GetUserByUsername(userCreate.Username)
	if appError == nil {
		addTeam(p, w, *userExist)
		userReturn := configuration.User{}.GetUser(userExist)
		jsonValue, _ := json.Marshal(userReturn)
		w.Write(jsonValue)
		return
	}
	userExist, appError = p.API.GetUserByEmail(userCreate.Email)
	if appError == nil {
		addTeam(p, w, *userExist)
		userReturn := configuration.User{}.GetUser(userExist)
		jsonValue, _ := json.Marshal(userReturn)
		w.Write(jsonValue)
		return
	}

	userCreated, appError := p.API.CreateUser(&userCreate)
	if appError != nil && appError.StatusCode != http.StatusOK {
		fmt.Fprintln(w, appError.ToJson())
		return
	}

	addTeam(p, w, *userCreated)
	userReturn := configuration.User{}.GetUser(userCreated)
	jsonValue, _ := json.Marshal(userReturn)
	w.Write(jsonValue)
}

func (p *MMPlugin) handleHello(writer http.ResponseWriter, request *http.Request) {
	fmt.Fprintln(writer, "Anvil listening")
}

func (p *MMPlugin) postMessage(c *plugin.Context, w http.ResponseWriter, r *http.Request) {
	rawBody, err := ioutil.ReadAll(r.Body)
	if err != nil {
		fmt.Fprintln(w, "Errror Geting body")
		return
	}

	incomingWebhookRequest := model.IncomingWebhookRequest{}
	incomingWebhook := model.IncomingWebhook{}
	err = json.Unmarshal(rawBody, &incomingWebhookRequest)
	if err != nil {
		fmt.Fprintln(w, "Errror Decoding Json user")
		return
	}
	err = json.Unmarshal(rawBody, &incomingWebhook)
	if err != nil {
		fmt.Fprintln(w, "Errror Decoding Json user")
		return
	}
	post := &model.Post{
		UserId:    incomingWebhook.UserId,
		ChannelId: incomingWebhook.ChannelId,
		Message:   incomingWebhookRequest.Text}

	if incomingWebhookRequest.Props != nil {
		post.Props = incomingWebhookRequest.Props
	}
	post.AddProp("attachments", incomingWebhookRequest.Attachments)
	p.API.SendEphemeralPost(post.UserId, post)

}

func addTeam(p *MMPlugin, w http.ResponseWriter, user model.User) {
	Client := model.NewAPIv4Client(configuration.MatterMostHost)
	Client.Login(configuration.MatterMostAdminUsername, configuration.MatterMostAdminPassword) //admin credencials
	teams, appError := p.API.GetTeams()
	if appError != nil {
		fmt.Fprintln(w, "Response: "+appError.ToJson())
		return
	}
	for _, team := range teams {
		_, response := Client.AddTeamMember(team.Id, user.Id)
		if response != nil && response.Error != nil {
			fmt.Fprintln(w, response.Error.ToJson())
		}
	}
}
