package bruxism

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math/rand"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/dustin/go-humanize"
)

// A Reminder holds data about a specific reminder.
type Reminder struct {
	StartTime time.Time
	Time      time.Time
	Requester string
	Target    string
	Message   string
	IsPrivate bool
}

// ReminderPlugin is a plugin that reminds users.
type ReminderPlugin struct {
	sync.RWMutex
	SimplePlugin
	bot       *Bot
	Reminders []*Reminder
}

var randomTimes = []string{
	"1 minute",
	"10 minutes",
	"1 hour",
	"4 hours",
	"tomorrow",
	"next week",
}

var randomMessages = []string{
	"walk the dog",
	"take pizza out of the oven",
	"check my email",
	"feed the baby",
}

func (p *ReminderPlugin) random(list []string) string {
	return list[rand.Intn(len(list))]
}

func (p *ReminderPlugin) randomReminder(service Service, command string) string {
	ticks := ""
	if service.Name() == DiscordServiceName {
		ticks = "`"
	}

	return fmt.Sprintf("%s%s %s | %s%s", ticks, command, p.random(randomTimes), p.random(randomMessages), ticks)
}

func (p *ReminderPlugin) helpFunc(bot *Bot, service Service, detailed bool) []string {
	if detailed {
		return []string{
			p.randomReminder(service, "remindme"),
			p.randomReminder(service, "remindchannel"),
		}
	}
	return []string{
		commandHelp(service, "remindme", "<time> | <reminder>", "Sets a reminder that is sent with a private message.")[0],
		commandHelp(service, "remindchannel", "<time> | <reminder>", "Sets a reminder that is sent to this channel.")[0],
	}
}

func (p *ReminderPlugin) parseTime(str string) (time.Time, error) {
	str = strings.ToLower(strings.Trim(str, " "))
	if str == "tomorrow" {
		return time.Now().Add(1 * time.Hour * 24), nil
	}

	split := strings.Split(str, " ")
	fmt.Println(split)
	if len(split) == 2 {
		if split[0] == "next" {
			switch split[1] {
			case "week":
				return time.Now().Add(1 * time.Hour * 24 * 7), nil
			case "month":
				return time.Now().Add(1 * time.Hour * 24 * 7 * 4), nil
			case "year":
				return time.Now().Add(1 * time.Hour * 24 * 365), nil
			default:
				return time.Time{}, errors.New("Invalid next.")
			}
		}

		i, err := strconv.Atoi(split[0])
		if err != nil {
			return time.Time{}, err
		}

		switch {
		case strings.HasPrefix(split[1], "second"):
			return time.Now().Add(time.Duration(i) * time.Second), nil
		case strings.HasPrefix(split[1], "minute"):
			return time.Now().Add(time.Duration(i) * time.Minute), nil
		case strings.HasPrefix(split[1], "hour"):
			return time.Now().Add(time.Duration(i) * time.Hour), nil
		}

	}
	return time.Time{}, errors.New("Invalid string.")
}

// AddReminder adds a reminder.
func (p *ReminderPlugin) AddReminder(reminder *Reminder) error {
	p.Lock()
	defer p.Unlock()

	i := 0
	for _, r := range p.Reminders {
		if r.Requester == reminder.Requester {
			i++
			if i > 5 {
				return errors.New("You have too many reminders already.")
			}
		}
	}

	i = 0
	for _, r := range p.Reminders {
		if r.Time.After(reminder.Time) {
			break
		}
		i++
	}

	p.Reminders = append(p.Reminders, reminder)
	copy(p.Reminders[i+1:], p.Reminders[i:])
	p.Reminders[i] = reminder

	return nil
}

func (p *ReminderPlugin) messageFunc(bot *Bot, service Service, message Message) {
	if !service.IsMe(message) {
		isPrivate := matchesCommand(service, "remindme", message)
		if isPrivate || matchesCommand(service, "remindchannel", message) {
			query, parts := parseCommand(service, message)

			var command string
			var target string
			if isPrivate {
				command = "remindme"
				target = message.UserID()
			} else {
				command = "remindchannel"
				target = message.Channel()
			}

			if len(parts) == 0 {
				service.SendMessage(message.Channel(), fmt.Sprintf("Invalid reminder, no time or message. eg: %s", p.randomReminder(service, command)))
				return
			}

			split := strings.Split(query, "|")
			if len(split) != 2 {
				service.SendMessage(message.Channel(), fmt.Sprintf("Invalid reminder. eg: %s", p.randomReminder(service, command)))
				return
			}

			t, err := p.parseTime(split[0])

			now := time.Now()

			if err != nil || t.Before(now) {
				service.SendMessage(message.Channel(), fmt.Sprintf("Invalid time. eg: %s", strings.Join(randomTimes, ", ")))
				return
			}

			requester := message.UserName()
			if service.Name() == DiscordServiceName {
				requester = fmt.Sprintf("<@%s>", message.UserID())
			}

			raw := strings.Split(message.RawMessage(), "|")

			err = p.AddReminder(&Reminder{
				StartTime: now,
				Time:      t,
				Requester: requester,
				IsPrivate: isPrivate,
				Target:    target,
				Message:   strings.Trim(raw[len(raw)-1], " "),
			})
			if err != nil {
				service.SendMessage(message.Channel(), err.Error())
				return
			}

			service.SendMessage(message.Channel(), fmt.Sprintf("Reminder set for %s.", humanize.Time(t)))
		}
	}
}

// ReminderMessage returns the message for a reminder.
func (p *ReminderPlugin) ReminderMessage(reminder *Reminder) string {
	if reminder.IsPrivate {
		return fmt.Sprintf("%s you set a reminder: %s", humanize.Time(reminder.StartTime), reminder.Message)
	}
	return fmt.Sprintf("%s %s set a reminder: %s", humanize.Time(reminder.StartTime), reminder.Requester, reminder.Message)
}

// SendReminder sends a reminder.
func (p *ReminderPlugin) SendReminder(service Service, reminder *Reminder) {
	if reminder.IsPrivate {
		service.PrivateMessage(reminder.Target, p.ReminderMessage(reminder))
	} else {
		service.SendMessage(reminder.Target, p.ReminderMessage(reminder))
	}
}

// Run will block until a reminder needs to be fired and then fire it.
func (p *ReminderPlugin) Run(bot *Bot, service Service) {
	for {
		p.RLock()

		if len(p.Reminders) > 0 {
			reminder := p.Reminders[0]
			if time.Now().After(reminder.Time) {
				p.SendReminder(service, reminder)
				p.Reminders = p.Reminders[1:]
				p.RUnlock()
				continue
			}
		}

		p.RUnlock()
		time.Sleep(time.Second)
	}
}

// Load will load plugin state from a byte array.
func (p *ReminderPlugin) Load(bot *Bot, service Service, data []byte) error {
	if data != nil {
		if err := json.Unmarshal(data, p); err != nil {
			log.Println("Error loading data", err)
		}
	}
	go p.Run(bot, service)
	return nil
}

// Save will save plugin state to a byte array.
func (p *ReminderPlugin) Save() ([]byte, error) {
	return json.Marshal(p)
}

// NewReminderPlugin will create a new Reminder plugin.
func NewReminderPlugin() Plugin {
	p := &ReminderPlugin{
		SimplePlugin: *NewSimplePlugin("Remind"),
		Reminders:    []*Reminder{},
	}
	p.message = p.messageFunc
	p.help = p.helpFunc
	return p
}