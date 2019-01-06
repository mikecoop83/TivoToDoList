import json
from .TivoToDoList import EpisodeDetails

tvmaze_ep = json.loads(
    '{"id":1574790,"url":"http://www.tvmaze.com/episodes/1574790/ray-donovan-6x11-never-gonna-give-you-up","name":"Never Gonna Give You Up","season":6,"number":11,"airdate":"2019-01-06","airtime":"21:00","airstamp":"2019-01-07T02:00:00+00:00","runtime":60,"image":null,"summary":null,"show":{"id":152,"url":"http://www.tvmaze.com/shows/152/ray-donovan","name":"Ray Donovan","type":"Scripted","language":"English","genres":["Drama","Action","Crime"],"status":"Running","runtime":60,"premiered":"2013-06-30","officialSite":"http://www.sho.com/sho/ray-donovan/home","schedule":{"time":"21:00","days":["Sunday"]},"rating":{"average":8.3},"weight":100,"network":{"id":9,"name":"Showtime","country":{"name":"United States","code":"US","timezone":"America/New_York"}},"webChannel":null,"externals":{"tvrage":30309,"thetvdb":259866,"imdb":"tt2249007"},"image":{"medium":"http://static.tvmaze.com/uploads/images/medium_portrait/166/415834.jpg","original":"http://static.tvmaze.com/uploads/images/original_untouched/166/415834.jpg"},"summary":"<p>Set in the sprawling mecca of the rich and famous, Ray Donovan does the dirty work for LA\'s top power players as the go-to guy who makes the problems of the city\'s celebrities, superstar athletes, and business moguls disappear. This powerful drama unfolds when his father is unexpectedly released from prison, setting off a chain of events that shakes the Donovan family to its core.</p>","updated":1546184379,"_links":{"self":{"href":"http://api.tvmaze.com/shows/152"},"previousepisode":{"href":"http://api.tvmaze.com/episodes/1560033"},"nextepisode":{"href":"http://api.tvmaze.com/episodes/1574790"}}},"_links":{"self":{"href":"http://api.tvmaze.com/episodes/1574790"}}}'
)


def test_parse_tvmaze():
    ed = EpisodeDetails.from_tvmaze_dict(tvmaze_ep)
    print(ed.to_html())
