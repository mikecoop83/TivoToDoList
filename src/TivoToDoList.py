#!/usr/bin/python
import sys
import json
import requests
import smtplib
from email.mime.text import MIMEText
import os

from datetime import datetime
from datetime import timedelta


def get_local_time(str_utc_time):
    if not str_utc_time:
        return None
    result = datetime.strptime(str_utc_time, "%Y-%m-%d %H:%M:%S") - offset
    return result


def sort_key(ep):
    return get_local_time(ep.get("requested_start_time"))


class EpisodeDetails:
    def __init__(self, ep):
        self.title = ep.get("title")
        self.subtitle = ep.get("subtitle")
        self.description = ep.get("description")
        self.requested_start_time = get_local_time(ep.get("requestedStartTime"))
        self.requested_end_time = get_local_time(ep.get("requestedEndTime"))
        self.url = "http://tvschedule.zap2it.com/tv/episode/" + ep.get(
            "partnerCollectionId"
        )

    def __repr__(self):
        return "%s: <b>%s</b> (<i>%s</i>) [%s]" % (
            self.requested_start_time.time(),
            self.title,
            self.subtitle or self.description or "Unknown",
            self.requested_end_time - self.requested_start_time,
        )


if __name__ == "__main__":
    offset = timedelta(
        hours=round(float((datetime.utcnow() - datetime.now()).seconds) / 60 / 60, 0)
    )

    with open("TivoToDoList.conf", "r") as configFile:
        config = json.loads(configFile.read())

    str_tivo_json = ""
    if os.path.isfile("toDoList.json"):
        file_size = os.stat("toDoList.json").st_size
        if file_size > 0:
            mod_time_sec = os.path.getmtime("toDoList.json")
            mod_time = datetime.fromtimestamp(mod_time_sec)
            if "-f" in sys.argv or mod_time.date() == datetime.now().date():
                with open("toDoList.json", "r") as tivo_json_file:
                    str_tivo_json = tivo_json_file.read()

    if not str_tivo_json:
        str_tivo_json = requests.get(
            config["kmttgBaseUrl"] + "/getToDo?tivo=Roamio"
        ).text

    list_tivo = json.loads(str_tivo_json)

    with open("toDoList.json", "w") as tivo_json_file:
        tivo_json_file.write(
            json.dumps(list_tivo, sort_keys=True, indent=4, separators=(",", ": "))
        )

    new_eps = [EpisodeDetails(ep) for ep in list_tivo if ep["isNew"]]

    todays_new_eps = [
        ep
        for ep in new_eps
        if ep.requested_start_time.date() == datetime.today().date()
    ]
    tomorrows_new_eps = [
        ep
        for ep in new_eps
        if ep.requested_start_time.date() == datetime.today().date() + timedelta(days=1)
    ]

    todays_new_eps.sort(key=lambda ep: ep.requested_start_time)
    tomorrows_new_eps.sort(key=lambda ep: ep.requested_start_time)

    message_str_list = ["Today's new episodes:"]
    for epd in todays_new_eps:
        message_str_list.append(str(epd))

    message_str_list.append("")
    message_str_list.append("Tomorrow's new episodes:")
    for epd in tomorrows_new_eps:
        message_str_list.append(str(epd))

    message = "<br/>\n".join(message_str_list)

    mime_message = MIMEText(message, "html")
    mime_message["Subject"] = "To do list for " + str(datetime.now().date())
    mime_message["From"] = config["smtp_name"]
    mime_message["To"] = ",".join(config["to_emails"])

    print(mime_message.as_string())

    if "-d" not in sys.argv:
        smtp_client = smtplib.SMTP(config["smtp_server"])
        smtp_client.ehlo()
        smtp_client.starttls()
        smtp_client.login(config["smtp_user"], config["smtp_password"])
        smtp_client.sendmail(
            config["smtp_user"], config["smtp_user"], mime_message.as_string()
        )
        smtp_client.close()
