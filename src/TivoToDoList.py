#!/usr/bin/python3
import dateutil.parser
import dateutil.tz
import json
import smtplib
import sys

from email.mime.text import MIMEText
from libtivomind import api

from datetime import date, datetime, timedelta
from typing import List


def get_local_time(str_utc_time: str) -> datetime:
    return (
        dateutil.parser.parse(str_utc_time)
        .replace(tzinfo=dateutil.tz.UTC)
        .astimezone(dateutil.tz.tzlocal())
    )


class EpisodeDetails:
    def __init__(self, ep):
        self.title = ep.get("title")
        self.subtitle = ep.get("subtitle")
        self.description = ep.get("description")
        self.requested_start_time = get_local_time(ep.get("requestedStartTime"))
        self.requested_end_time = get_local_time(ep.get("requestedEndTime"))

    def to_html(self) -> str:
        return "%s: <b>%s</b> (<i>%s</i>) [%s]" % (
            self.requested_start_time.time().strftime("%I:%M %p"),
            self.title,
            self.subtitle or self.description or "Unknown",
            str((self.requested_end_time - self.requested_start_time))[:-3],
        )


if __name__ == "__main__":
    with open("TivoToDoList.conf", "r") as configFile:
        config = json.loads(configFile.read())

    mind = api.Mind.new_local_session(
        cert_path=config["cert_path"],
        cert_password=config["cert_password"],
        address=config["tivo_ip"],
        mak=config["tivo_mak"],
        port=config["tivo_port"],
    )

    to_do_list = mind.recording_search(
        fetch_all=True, filt={"state": ["inProgress", "scheduled"]}
    )

    new_eps = [EpisodeDetails(ep) for ep in to_do_list if ep["isNew"]]

    def get_new_eps_by_date(start_date: date) -> List[EpisodeDetails]:
        result = [ep for ep in new_eps if ep.requested_start_time.date() == start_date]
        result.sort(key=lambda ep: ep.requested_start_time)
        return result

    todays_new_eps = get_new_eps_by_date(datetime.today().date())
    tomorrows_new_eps = get_new_eps_by_date(datetime.today().date() + timedelta(days=1))

    message_str_list = ["Today's new episodes:"] + [
        ep.to_html() for ep in todays_new_eps
    ]
    message_str_list += [""]
    message_str_list += ["Tomorrow's new episodes:"] + [
        ep.to_html() for ep in tomorrows_new_eps
    ]

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
