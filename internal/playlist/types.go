package playlist

type URLSelectParams struct {
	Mode           string
	IGMPHTTPPrefix string
	R2HBaseURL     string
	R2HIGMPPath    string
	R2HToken       string
	R2HAddFCC      bool
	R2HFCCTYPE     string
	R2HProxyRTSP   bool
}

type Channel struct {
	Name            string
	URL             string
	UserChannelID   string
	TimeShiftURL    string
	TimeShiftDays   int
	TimeShiftLength int
}

type Reference struct {
	Name    string
	EXTINF  string
	Options []string
}

type Row struct {
	Name    string
	URL     string
	EPGID   string
	EPGName string
	LogoURL string
	Ref     *Reference
}

type RenderOptions struct {
	DisplayNameMode string
	XTvgURL         string
	GroupNames      map[string]string
	Catchup         map[string]Catchup
	TimeShiftLength map[string]int
	CatchupType     string
}

type Catchup struct {
	Source string
	Days   int
}
