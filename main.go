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
	Title       string
	Subtitle    string
	Description string
	StartTime   time.Time
	EndTime     time.Time
}

func atMidnight(dateTime time.Time) time.Time {
	return time.Date(
		dateTime.Year(),
		dateTime.Month(),
		dateTime.Day(),
		0, 0, 0, 0,
		dateTime.Location())
}

func parseTivoTime(utcTime string) time.Time {
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
	title := ep.MustString("title")
	subtitle := ep.MustString("subtitle")
	description, _ := ep.String("description")
	requestedStartTime := ep.MustString("requestedStartTime")
	requestedEndTime := ep.MustString("requestedEndTime")
	return episodeDetails{
		Title:       title,
		Subtitle:    subtitle,
		Description: description,
		StartTime:   parseTivoTime(requestedStartTime),
		EndTime:     parseTivoTime(requestedEndTime),
	}
}

func firstNonEmpty(strs ...string) string {
	for _, s := range strs {
		if s != "" {
			return s
		}
	}
	return ""
}

func (ep episodeDetails) toHtml() string {
	return fmt.Sprintf(
		"%s: <b>%s</b> (<i>%s</i>) [%s]",
		ep.StartTime.Format("03:04 PM"),
		ep.Title,
		firstNonEmpty(ep.Subtitle, ep.Description, "Unknown"),
		strings.TrimRight(ep.EndTime.Sub(ep.StartTime).String(), "0s"),
	)
}

func episodeFromTVMazeMap(ep json.Map) episodeDetails {
	ed := episodeDetails{}
	show := ep.Map("show")
	ed.Title = show.MustString("name")
	ed.Subtitle = ep.MustString("name")
	ed.StartTime, _ = time.Parse("2006-01-02T15:04:05+00:00", ep.MustString("airstamp"))
	ed.StartTime = ed.StartTime.UTC().In(time.Local)
	ed.EndTime = ed.StartTime.Add(time.Duration(int(ep.MustFloat("runtime"))) * time.Minute)
	return ed
}

func episodeFromTVMazeWebMap(ep json.Map) episodeDetails {
	ed := episodeDetails{}
	show := ep.Map("_embedded").Map("show")
	ed.Title = show.MustString("name")
	ed.Subtitle = ep.MustString("name")
	ed.StartTime, _ = time.Parse("2006-01-02T15:04:05+00:00", ep.MustString("airstamp"))
	ed.StartTime = ed.StartTime.UTC().In(time.Local)
	ed.EndTime = ed.StartTime.Add(time.Duration(int(ep.MustFloat("runtime"))) * time.Minute)
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
		urls := []string{
			fmt.Sprintf("https://api.tvmaze.com/schedule?date=%s", dateFmt),
			fmt.Sprintf("https://api.tvmaze.com/schedule/web?date=%s", dateFmt),
		}
		for _, url := range urls {
			response, err := http.DefaultClient.Get(url)
			if err != nil {
				return nil, err
			}
			tvMazeGuide := json.ArrayFromReader(response.Body)
			_ = response.Body.Close()
			if err != nil {
				return nil, err
			}
			for i := 0; i < tvMazeGuide.MustLen(); i++ {
				var web bool
				var showID int
				ep := tvMazeGuide.Map(i)
				if ep.MustHas("_embedded") {
					showID = int(ep.Map("_embedded").Map("show").MustFloat("id"))
					web = true
				} else {
					showID = int(ep.Map("show").MustFloat("id"))
				}
				if _, ok := showMap[showID]; ok {
					var ed episodeDetails
					if web {
						ed = episodeFromTVMazeWebMap(ep)
					} else {
						ed = episodeFromTVMazeMap(ep)
					}
					results = append(results, ed)
				}
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
	tivoArray := json.NewArray(tivoList)
	for i := 0; i < len(tivoList); i++ {
		tivoMap := tivoArray.Map(i)
		if tivoMap.MustBool("isNew") {
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

	nomail := flag.Bool("nomail", false, "do everything except sending the mail")
	runDate := flag.String("date", "", "run for this date")
	flag.Parse()

	var today time.Time
	var err error
	if *runDate != "" {
		today, err = time.Parse("2006-01-02", *runDate)
		if err != nil {
			return err
		}
	} else {
		today = atMidnight(time.Now())
	}
	tomorrow := today.AddDate(0, 0, 1)

	config := json.MapFromBytes(configBytes)
	host := config.MustString("tivo_ip")
	port := int(config.MustFloat("tivo_port"))
	mak := config.MustString("tivo_mak")

	tivoEps, err := getTivoEpisodes(today, tomorrow, host, port, mak)
	if err != nil {
		return err
	}

	dates := []time.Time{today, tomorrow}

	srv, err := sheets.NewService(context.Background(), option.WithAPIKey(config.MustString("google_api_key")))
	if err != nil {
		return err
	}
	spreadsheetId, _ := config.String("google_tvmaze_sheet_id")
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
		epDate := atMidnight(ep.StartTime)
		if epDate == today {
			todaysNewEps = append(todaysNewEps, ep)
		} else if epDate == tomorrow {
			tomorrowsNewEps = append(tomorrowsNewEps, ep)
		}
	}

	sort.Slice(todaysNewEps, func(i, j int) bool {
		return todaysNewEps[i].StartTime.Unix() < todaysNewEps[j].StartTime.Unix()
	})
	sort.Slice(tomorrowsNewEps, func(i, j int) bool {
		return tomorrowsNewEps[i].StartTime.Unix() < tomorrowsNewEps[j].StartTime.Unix()
	})

	messageBody := generateMessageBody(todaysNewEps, tomorrowsNewEps)
	fmt.Println(messageBody)

	m := &gophermail.Message{}
	err = m.SetFrom(config.MustString("smtp_name") + " <" + config.MustString("smtp_user") + ">")
	if err != nil {
		return err
	}
	toEmails := config.Array("to_emails")
	var recipients []string
	for i := 0; i < toEmails.MustLen(); i++ {
		toEmail := toEmails.MustString(i)
		err := m.AddTo(toEmail)
		if err != nil {
			return err
		}
		recipients = append(recipients, toEmail)
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
		auth := smtp.PlainAuth("", config.MustString("smtp_user"), config.MustString("smtp_password"), config.MustString("smtp_host"))
		err = smtp.SendMail(config.MustString("smtp_server"), auth, config.MustString("smtp_name"), recipients, msgBytes)
		if err != nil {
			return err
		}
	}
	return nil
}

func generateMessageBody(todaysNewEps, tomorrowsNewEps []episodeDetails) string {
	var messageList []string
	messageList = append(messageList, "Today's new episodes:")
	for _, ep := range todaysNewEps {
		messageList = append(messageList, ep.toHtml())
	}
	messageList = append(messageList, "", "Tomorrow's new episodes")
	for _, ep := range tomorrowsNewEps {
		messageList = append(messageList, ep.toHtml())
	}
	messageBody := strings.Join(messageList, "<br/>")
	return messageBody
}
