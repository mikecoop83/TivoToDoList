package main

import (
	"context"
	_ "embed"
	"flag"
	"fmt"
	"log"
	"net/http"
	"net/mail"
	"net/smtp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/mikecoop83/json"
	"github.com/mikecoop83/tivo"
	"google.golang.org/api/option"
	"google.golang.org/api/sheets/v4"
	"gopkg.in/jpoehls/gophermail.v0"
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

func episodeFromTivoMap(ep json.Map) episodeDetails {
	title := ep.String("title")
	subtitle := ep.String("subtitle")
	description := ep.String("description")
	requestedStartTime := ep.String("requestedStartTime")
	requestedEndTime := ep.String("requestedEndTime")
	return episodeDetails{
		Title:              title,
		Subtitle:           subtitle,
		Description:        description,
		RequestedStartTime: parseUtcAsLocalTime(requestedStartTime),
		RequestedEndTime:   parseUtcAsLocalTime(requestedEndTime),
	}
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

func episodeFromTVMazeMap(ep json.Map) episodeDetails {
	ed := episodeDetails{}
	show := ep.Map("show")
	ed.Title = show.String("name")
	ed.Subtitle = ep.String("name")
	ed.RequestedStartTime, _ = time.Parse("2006-01-02T15:04:05+00:00", ep.String("airstamp"))
	ed.RequestedStartTime.UTC().In(time.Local)
	ed.RequestedEndTime = ed.RequestedStartTime.Add(time.Duration(ep.Int("runtime")) * time.Minute)
	return ed
}

func getTVMazeShows(dates []time.Time, showIDs []int) ([]episodeDetails, error) {
	results := make([]episodeDetails, 0, len(showIDs)*2)
	showMap := make(map[int]struct{}, len(showIDs))
	for _, showID := range showIDs {
		showMap[showID] = struct{}{}
	}
	for _, date := range dates {
		dateFmt := date.Format("2006-01-02")
		url := fmt.Sprintf("https://api.tvmaze.com/schedule?date=%s", dateFmt)
		response, err := http.DefaultClient.Get(url)
		if err != nil {
			return nil, err
		}
		defer response.Body.Close()
		tvMazeGuide, err := json.FromReader[json.Array](response.Body)
		if err != nil {
			return nil, err
		}
		for i := 0; i < len(tvMazeGuide); i++ {
			ep := tvMazeGuide.Map(i)
			showID := ep.Map("show").Int("id")
			if _, ok := showMap[showID]; ok {
				ed := episodeFromTVMazeMap(ep)
				results = append(results, ed)
			}
		}
	}
	return results, nil
}

func getTivoEpisodes(minDate, maxDate time.Time, host string, port int, mak string) ([]episodeDetails, error) {
	results := make([]episodeDetails, 0, 10)
	session := tivo.NewSession(host, port, tivo.MakCredential{Mak: mak}, false)
	err := session.Connect()
	if err != nil {
		return nil, err
	}

	tivoList, err := session.RecordingSearch(
		map[string]interface{}{
			"state":        []interface{}{"scheduled"},
			"minStartTime": minDate.Local().In(time.UTC).Format("2006-01-02 15:04:05"),
			"maxStartTime": maxDate.Add(24*time.Hour - time.Microsecond).Local().In(time.UTC).Format("2006-01-02 15:04:05"),
		},
	)
	if err != nil {
		return nil, err
	}
	tivoArray := json.Array(tivoList)
	for i := 0; i < len(tivoList); i++ {
		tivoMap := tivoArray.Map(i)
		if tivoMap.Bool("isNew") {
			ed := episodeFromTivoMap(tivoMap)
			results = append(results, ed)
		}
	}
	return results, nil
}

func main() {
	if err := run(); err != nil {
		panic(err)
	}
}

//go:embed resources/TivoToDoList.conf
var configBytes []byte

func run() error {
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)

	today := getDate(time.Now())
	tomorrow := today.AddDate(0, 0, 1)

	nomail := flag.Bool("nomail", false, "do everything except sending the mail")

	flag.Parse()

	config, err := json.FromBytes[json.Map](configBytes)
	if err != nil {
		return err
	}
	host := config.String("tivo_ip")
	port := config.Int("tivo_port")
	mak := config.String("tivo_mak")

	tivoEps, err := getTivoEpisodes(today, tomorrow, host, port, mak)
	if err != nil {
		return err
	}

	dates := []time.Time{today, tomorrow}

	srv, err := sheets.NewService(context.Background(), option.WithAPIKey(config.String("google_api_key")))
	if err != nil {
		return err
	}
	spreadsheetId := config.String("google_tvmaze_sheet_id")
	readRange := "A:A"
	resp, err := srv.Spreadsheets.Values.Get(spreadsheetId, readRange).Do()
	if err != nil {
		return err
	}
	tvMazeShowIDs := make([]int, 0, len(resp.Values))
	for _, rows := range resp.Values {
		showID, err := strconv.Atoi(rows[0].(string))
		if err != nil {
			return err
		}
		tvMazeShowIDs = append(tvMazeShowIDs, showID)
	}
	tvMazeEps, err := getTVMazeShows(dates, tvMazeShowIDs)
	if err != nil {
		return err
	}

	episodes := append(tivoEps, tvMazeEps...)

	var todaysNewEps, tomorrowsNewEps []episodeDetails
	for _, ep := range episodes {
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

	fmt.Println(messageBody)

	m := &gophermail.Message{}
	err = m.SetFrom(config.String("smtp_name") + " <" + config.String("smtp_user") + ">")
	if err != nil {
		return err
	}
	toEmails := config.Array("to_emails")
	var recipients []string
	for _, toEmail := range toEmails {
		err := m.AddTo(toEmail.(string))
		if err != nil {
			return err
		}
		recipients = append(recipients, toEmail.(string))
	}

	m.Subject = "To do list for " + time.Now().Format("2006-01-02")
	m.HTMLBody = messageBody
	m.Headers = mail.Header{}
	m.Headers["Date"] = []string{time.Now().UTC().Format(time.RFC822)}

	msgBytes, err := m.Bytes()
	if err != nil {
		return err
	}

	fmt.Printf("%s", msgBytes)

	if !(*nomail) {
		auth := smtp.PlainAuth("", config.String("smtp_user"), config.String("smtp_password"), config.String("smtp_host"))
		err = smtp.SendMail(config.String("smtp_server"), auth, config.String("smtp_name"), recipients, msgBytes)
		if err != nil {
			return err
		}
	}
	return nil
}
