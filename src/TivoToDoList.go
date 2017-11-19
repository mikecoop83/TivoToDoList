package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/mail"
	"net/smtp"
	"os"
	"sort"
	"strings"
	"time"

	gophermail "gopkg.in/jpoehls/gophermail.v0"
)

type episodeDetails struct {
	Title              string
	Subtitle           string
	Description        string
	RequestedStartTime time.Time
	RequestedEndTime   time.Time
}

func getDate(dateTime time.Time) time.Time {
	return time.Date(
		dateTime.Year(),
		dateTime.Month(),
		dateTime.Day(),
		0, 0, 0, 0,
		dateTime.Location())
}

func parseUtcAsLocalTime(utcTime string) time.Time {
	if utcTime == "" {
		return time.Time{}
	}
	result, err := time.Parse("2006-01-02 15:04:05", utcTime)
	if err != nil {
		panic(err)
	}
	return result.In(time.Now().Location())
}

func newEpisodeDetails(episodeMap map[string]interface{}) episodeDetails {
	title, _ := episodeMap["title"].(string)
	subtitle, _ := episodeMap["subtitle"].(string)
	description, _ := episodeMap["description"].(string)
	requestedStartTime, _ := episodeMap["requestedStartTime"].(string)
	requestedEndTime, _ := episodeMap["requestedEndTime"].(string)
	ep := episodeDetails{
		Title:              title,
		Subtitle:           subtitle,
		Description:        description,
		RequestedStartTime: parseUtcAsLocalTime(requestedStartTime),
		RequestedEndTime:   parseUtcAsLocalTime(requestedEndTime),
	}
	return ep
}

func coalesce(objs ...interface{}) interface{} {
	if objs == nil {
		return nil
	}
	for _, obj := range objs {
		if obj != nil {
			return obj
		}
	}
	return nil
}

func (ep episodeDetails) String() string {
	return fmt.Sprintf("%s: <b>%s</b> (<i>%s</i>) [%s]",
		ep.RequestedStartTime.Format("03:04 PM"), ep.Title, coalesce(ep.Subtitle, ep.Description, "Unknown"), strings.TrimRight(ep.RequestedEndTime.Sub(ep.RequestedStartTime).String(), "0s"))
}

func main() {
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)

	nomail := flag.Bool("nomail", false, "do everything except sending the mail")
	flag.Parse()

	var err error
	var configBuf []byte
	if configBuf, err = ioutil.ReadFile("TivoToDoList.conf"); err != nil {
		panic(err)
	}
	var configParsed interface{}
	if err = json.Unmarshal(configBuf, &configParsed); err != nil {
		panic(err)
	}
	config := configParsed.(map[string]interface{})
	var tivoJSON []byte
	info, _ := os.Stat("toDoList.json")
	if info != nil && getDate(info.ModTime()) == getDate(time.Now()) {
		if tivoJSON, err = ioutil.ReadFile("toDoList.json"); err != nil {
			panic(err)
		}
	}

	if tivoJSON == nil {
		baseURL := config["kmttgBaseUrl"].(string)
		kmttgURL := baseURL + "/getToDo?tivo=Roamio"

		log.Printf("Making request to %s", kmttgURL)
		var resp *http.Response
		if resp, err = http.Get(kmttgURL); err != nil {
			panic(err)
		}
		defer resp.Body.Close()
		log.Printf("Got response with %d bytes", resp.ContentLength)
		if tivoJSON, err = ioutil.ReadAll(resp.Body); err != nil {
			panic(err)
		}
		log.Print("Read response body")
		if err = ioutil.WriteFile("toDoList.json", tivoJSON, 0644); err != nil {
			panic(err)
		}
		log.Print("Wrote response body to file")
	}

	log.Printf("Parsing response body to map")
	var tivoJSONParsed interface{}
	err = json.Unmarshal(tivoJSON, &tivoJSONParsed)
	if err != nil {
		panic(err)
	}
	tivoList := tivoJSONParsed.([]interface{})
	log.Printf("Parsed response body to map")
	var newEps []episodeDetails
	for _, ep := range tivoList {
		epMap := ep.(map[string]interface{})
		if epMap["isNew"].(bool) {
			newEps = append(newEps, newEpisodeDetails(epMap))
		}
	}
	log.Printf("Found %d new episodes", len(newEps))

	var todaysNewEps, tomorrowsNewEps []episodeDetails
	today := getDate(time.Now())
	tomorrow := today.AddDate(0, 0, 1)
	for _, ep := range newEps {
		epDate := getDate(ep.RequestedStartTime)
		if epDate == today {
			todaysNewEps = append(todaysNewEps, ep)
		} else if epDate == tomorrow {
			tomorrowsNewEps = append(tomorrowsNewEps, ep)
		}
	}

	sort.Slice(todaysNewEps, func(i, j int) bool {
		return todaysNewEps[i].RequestedStartTime.Unix() < todaysNewEps[j].RequestedStartTime.Unix()
	})
	sort.Slice(tomorrowsNewEps, func(i, j int) bool {
		return tomorrowsNewEps[i].RequestedStartTime.Unix() < tomorrowsNewEps[j].RequestedStartTime.Unix()
	})

	var messageList []string
	messageList = append(messageList, "Today's new episodes:")
	for _, ep := range todaysNewEps {
		messageList = append(messageList, ep.String())
	}

	messageList = append(messageList, "")

	messageList = append(messageList, "Tomorrow's new episodes")
	for _, ep := range tomorrowsNewEps {
		messageList = append(messageList, ep.String())
	}

	messageBody := strings.Join(messageList, "<br/>")

	m := &gophermail.Message{}
	m.SetFrom(config["smtp_name"].(string) + " <" + config["smtp_user"].(string) + ">")
	toEmails := config["to_emails"].([]interface{})
	var recipients []string
	for _, toEmail := range toEmails {
		m.AddTo(toEmail.(string))
		recipients = append(recipients, toEmail.(string))
	}

	m.Subject = "To do list for " + time.Now().Format("2006-01-02")
	m.HTMLBody = messageBody
	m.Headers = mail.Header{}
	m.Headers["Date"] = []string{time.Now().UTC().Format(time.RFC822)}

	msgBytes, err := m.Bytes()
	if err != nil {
		panic(err)
	}

	fmt.Printf("%s", msgBytes)

	if !(*nomail) {
		auth := smtp.PlainAuth("", config["smtp_user"].(string), config["smtp_password"].(string), config["smtp_host"].(string))
		err = smtp.SendMail(config["smtp_server"].(string), auth, config["smtp_name"].(string), recipients, msgBytes)
		if err != nil {
			panic(err)
		}
	}
}
