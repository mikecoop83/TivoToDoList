#!/usr/bin/python
import sys
import json
import urllib
import smtplib
from email.mime.text import MIMEText
import os

from datetime import datetime
from datetime import timedelta

class EpisodeDetails:
    def __init__(self, ep):
        self.title = ep.get('title')
        self.subtitle = ep.get('subtitle')
        self.description = ep.get('description')
        self.requestedStartTime = getLocalTime(ep.get('requestedStartTime'))
        self.requestedEndTime = getLocalTime(ep.get('requestedEndTime'))
        self.url = 'http://tvschedule.zap2it.com/tv/episode/' + ep.get('partnerCollectionId')

    def __repr__(self):
        return "%s: <b><a href='%s'>%s</a></b> (<i>%s</i>) [%s]" % (self.requestedStartTime.time(), self.url, self.title, self.subtitle or self.description or "Unknown", self.requestedEndTime - self.requestedStartTime)

def sortKey(ep):
    return getLocalTime(ep.get('requestedStartTime'))

def getLocalTime(strUtcTime):
    if not strUtcTime:
        return None
    result = datetime.strptime(strUtcTime, '%Y-%m-%d %H:%M:%S') - offset
    return result

offset = timedelta(hours=round(float((datetime.utcnow() - datetime.now()).seconds) / 60 / 60, 0))

with open('TivoToDoList.conf', 'r') as configFile:
    config = json.loads(configFile.read())

strTivoJson = ''
if os.path.isfile('toDoList.json'):
    fileSize = os.stat('toDoList.json').st_size
    if fileSize > 0:
        modTimeInSec = os.path.getmtime('toDoList.json')
        modTime = datetime.fromtimestamp(modTimeInSec)
        if "-f" in sys.argv or modTime.date() == datetime.now().date():
            with open('toDoList.json', 'r') as tivoJsonFile:
                strTivoJson = tivoJsonFile.read()

if not strTivoJson:
    strTivoJson = urllib.urlopen(config['kmttgBaseUrl'] + '/getToDo?tivo=Roamio').read()

listTivo = json.loads(strTivoJson)

with open('toDoList.json', 'w') as tivoJsonFile:
        tivoJsonFile.write(json.dumps(listTivo, sort_keys=True, indent=4, separators=(',', ': ')))

newEps = [EpisodeDetails(ep) for ep in listTivo if ep['isNew']]

todaysNewEps = [ep for ep in newEps if ep.requestedStartTime.date() == datetime.today().date()];
tomorrowsNewEps = [ep for ep in newEps if ep.requestedStartTime.date() == datetime.today().date() + timedelta(days=1)];

todaysNewEps.sort(key=lambda ep: ep.requestedStartTime)
tomorrowsNewEps.sort(key=lambda ep: ep.requestedStartTime)

message_str_list = []

message_str_list.append("Today's new episodes:")
for epd in todaysNewEps:
    message_str_list.append(str(epd))

message_str_list.append("")
message_str_list.append("Tomorrow's new episodes:")
for epd in tomorrowsNewEps:
    message_str_list.append(str(epd))

message = '<br/>\n'.join(message_str_list)

mimeMessage = MIMEText(message, 'html')
mimeMessage['Subject'] = 'To do list for ' + str(datetime.now().date())
mimeMessage['From'] = config['smtp_name']
mimeMessage['To'] = ','.join(config['to_emails'])

print(mimeMessage.as_string())

smtpClient = smtplib.SMTP(config['smtp_server'])
smtpClient.ehlo()
smtpClient.starttls()
smtpClient.login(config['smtp_user'], config['smtp_password'])
smtpClient.sendmail(config['smtp_user'], config['smtp_user'], mimeMessage.as_string())
smtpClient.close()
