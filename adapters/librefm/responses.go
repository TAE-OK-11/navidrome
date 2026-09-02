package librefm

type Response struct {
	Error      int        `json:"error"`
	Message    string     `json:"message"`
	Token      string     `json:"token"`
	Session    Session    `json:"session"`
	NowPlaying NowPlaying `json:"nowplaying"`
	Scrobbles  Scrobbles  `json:"scrobbles"`
}

type Session struct {
	Name       string `json:"name"`
	Key        string `json:"key"`
	Subscriber int    `json:"subscriber"`
}

type NowPlaying struct {
	IgnoredMessage struct {
		Code string `json:"code"`
		Text string `json:"#text"`
	} `json:"ignoredMessage"`
}

type Scrobbles struct {
	Attr struct {
		Accepted int `json:"accepted"`
		Ignored  int `json:"ignored"`
	} `json:"@attr"`
	Scrobble struct {
		IgnoredMessage struct {
			Code string `json:"code"`
			Text string `json:"#text"`
		} `json:"ignoredMessage"`
	} `json:"scrobble"`
}
