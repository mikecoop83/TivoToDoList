#!/usr/bin/env python3
import json
import logging
import os
import requests
import smtplib
import sys
import pytz
from tzlocal import get_localzone
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


class EpisodeDetails(object):
    __slots__ = [
        "title",
        "subtitle",
        "description",
        "requested_start_time",
        "requested_end_time",
    ]

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


class ToDoListRetriever(object):
    def __init__(self, config):
        self.config = config

    def _get_new_eps_by_date(
        self, new_eps: List[EpisodeDetails], start_date: date
    ) -> List[EpisodeDetails]:
        result = [ep for ep in new_eps if ep.requested_start_time.date() == start_date]
        result.sort(key=lambda ep: ep.requested_start_time)
        return result

    def get_new_episodes(self, dates: List[date]) -> Dict[date, List[EpisodeDetails]]:
        logging.info(
            f"Connecting to tivo at {self.config['tivo_ip']}:{self.config['tivo_port']} using cert at {os.path.abspath(self.config['cert_path'])}"
        )

        mind = api.Mind.new_local_session(
            cert_path=self.config["cert_path"],
            cert_password=self.config["cert_password"],
            address=self.config["tivo_ip"],
            mak=self.config["tivo_mak"],
            port=self.config["tivo_port"],
        )

        local_tz = get_localzone()
        logging.info(f"Querying tivo for to do list")
        min_date_time = (
            datetime.combine(min(dates), datetime.min.time())
            .replace(tzinfo=local_tz)
            .astimezone(pytz.utc)
        )
        max_date_time = (
            datetime.combine(max(dates), datetime.max.time())
            .replace(tzinfo=local_tz)
            .astimezone(pytz.utc)
        )
        to_do_list = mind.recording_search(
            fetch_all=True,
            filt={
                "state": ["scheduled"],
                "minStartTime": min_date_time.strftime(
                    "%Y-%m-%d %H:%M:%S"
                ),
                "maxStartTime": max_date_time.strftime(
                    "%Y-%m-%d %H:%M:%S"
                )
            },
        )

        logging.debug("Finding new episodes")
        new_eps = [
            EpisodeDetails.from_tivo_dict(ep) for ep in to_do_list if ep["isNew"]
        ]

        tvmaze_show_ids = set(config.get("tvmaze_show_ids", []))
        if tvmaze_show_ids:
            logging.info(f"Querying tvmaze for schedule for {dates}")
            for date in dates:
                tvmaze_url = f"http://api.tvmaze.com/schedule?date={date}"
                guide = requests.get(tvmaze_url).json()
                new_eps += [
                    EpisodeDetails.from_tvmaze_dict(ep)
                    for ep in guide
                    if ep.get("show", {}).get("id") in tvmaze_show_ids
                ]

        return {date: self._get_new_eps_by_date(new_eps, date) for date in dates}


if __name__ == "__main__":
    logging.basicConfig(
        format="%(asctime)s [%(levelname)s] %(message)s", level=logging.DEBUG
    )
    config_filename = "TivoToDoList.conf"
    logging.debug(f"Loading config file from {os.path.abspath(config_filename)}")
    with open(config_filename, "r") as configFile:
        config = json.loads(configFile.read())

    today = datetime.today().date()
    dates_and_labels = [(today, "Today"), (today + timedelta(days=1), "Tomorrow")]

    retriever = ToDoListRetriever(config)
    new_eps_by_date = retriever.get_new_episodes([dl[0] for dl in dates_and_labels])

    message_list = []
    for date, label in dates_and_labels:
        message_list += [f"{label}'s new episodes:"] + [
            ep.to_html() for ep in new_eps_by_date.get(date, [])
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
