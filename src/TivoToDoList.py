#!/usr/bin/env python3
import json
import logging
import os
import requests
import smtplib
import sys
from bs4 import BeautifulSoup
from datetime import date, datetime, timedelta
from email.mime.text import MIMEText
from typing import List, Dict

import dateutil.parser
import dateutil.tz
from libtivomind import api


def local_datetime_from_utc_string(str_utc_time: str) -> datetime:
    return (
        dateutil.parser.parse(str_utc_time)
        .replace(tzinfo=dateutil.tz.UTC)
        .astimezone(dateutil.tz.tzlocal())
    )


class EpisodeDetails:
    @classmethod
    def from_tivo_dict(cls, ep: Dict):
        ed = EpisodeDetails()
        ed.title = ep.get("title")
        ed.subtitle = ep.get("subtitle")
        ed.description = ep.get("description")
        ed.requested_start_time = local_datetime_from_utc_string(
            ep.get("requestedStartTime")
        )
        ed.requested_end_time = local_datetime_from_utc_string(
            ep.get("requestedEndTime")
        )
        return ed

    @classmethod
    def from_tvmaze_dict(cls, ep: Dict):
        ed = EpisodeDetails()
        ed.title = ep["show"]["name"]
        ed.subtitle = ep["name"]
        ed.description = ep["show"]["summary"]
        if ed.description:
            ed.description = BeautifulSoup(ed.description, "html.parser").get_text()
        ed.requested_start_time = local_datetime_from_utc_string(ep["airstamp"])
        ed.requested_end_time = ed.requested_start_time + timedelta(
            minutes=ep["runtime"]
        )
        return ed

    def to_html(self) -> str:
        subtitle = self.subtitle or self.description or "Unknown"
        ep_length = str((self.requested_end_time - self.requested_start_time))[:-3]
        return f"{self.requested_start_time:%I:%M %p}: <b>{self.title}</b> (<i>{subtitle}</i>) [{ep_length}]"


if __name__ == "__main__":
    logging.basicConfig(
        format="%(asctime)s [%(levelname)s] %(message)s", level=logging.DEBUG
    )
    config_filename = "TivoToDoList.conf"
    logging.debug(f"Loading config file from {os.path.abspath(config_filename)}")
    with open(config_filename, "r") as configFile:
        config = json.loads(configFile.read())

    logging.info(
        f"Connecting to tivo at {config['tivo_ip']}:{config['tivo_port']} using cert at {os.path.abspath(config['cert_path'])}"
    )
    mind = api.Mind.new_local_session(
        cert_path=config["cert_path"],
        cert_password=config["cert_password"],
        address=config["tivo_ip"],
        mak=config["tivo_mak"],
        port=config["tivo_port"],
    )

    logging.info(f"Querying tivo for to do list")
    to_do_list = mind.recording_search(
        fetch_all=True, filt={"state": ["inProgress", "scheduled"]}
    )

    today = datetime.today().date()
    dates_and_labels = [(today, "Today"), (today + timedelta(days=1), "Tomorrow")]

    logging.debug("Finding new episodes")
    new_eps = [EpisodeDetails.from_tivo_dict(ep) for ep in to_do_list if ep["isNew"]]

    tvmaze_eps = []
    try:
        tvmaze_show_ids = set(config.get("tvmaze_show_ids", []))
        if tvmaze_show_ids:
            logging.info(f"Querying tvmaze for schedule for {dates_and_labels}")
            for date, _ in dates_and_labels:
                tvmaze_url = f"http://api.tvmaze.com/schedule?date={date}"
                guide = requests.get(tvmaze_url).json()
                new_eps += [
                    EpisodeDetails.from_tvmaze_dict(ep)
                    for ep in guide
                    if ep.get("show", {}).get("id") in tvmaze_show_ids
                ]
    except Exception:
        logging.exception("Error querying tvmaze for schedule")

    def get_new_eps_by_date(start_date: date) -> List[EpisodeDetails]:
        result = [ep for ep in new_eps if ep.requested_start_time.date() == start_date]
        result.sort(key=lambda ep: ep.requested_start_time)
        return result

    message_list = []
    for date, label in dates_and_labels:
        message_list += [f"{label}'s new episodes:"] + [
            ep.to_html() for ep in get_new_eps_by_date(date)
        ]
        message_list += [""]

    message = "<br/>\n".join(message_list)

    mime_message = MIMEText(message, "html")
    mime_message["Subject"] = "To do list for " + str(datetime.now().date())
    mime_message["From"] = config["smtp_name"]
    mime_message["To"] = ",".join(config["to_emails"])

    if "-d" in sys.argv:
        logging.info(f"Not sending email: {mime_message}")
    else:
        logging.info(f"Sending email: {mime_message}")
        smtp_client = smtplib.SMTP(config["smtp_server"])
        smtp_client.ehlo()
        smtp_client.starttls()
        smtp_client.login(config["smtp_user"], config["smtp_password"])
        smtp_client.sendmail(
            config["smtp_user"], config["to_emails"], mime_message.as_string()
        )
        smtp_client.close()

    logging.info("Done")
