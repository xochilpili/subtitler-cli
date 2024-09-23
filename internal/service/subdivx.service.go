package service

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"

	"sync"

	"github.com/fatih/color"
	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/microcosm-cc/bluemonday"
	file "github.com/xochilpili/subtitler-cli/internal/files"
	"github.com/xochilpili/subtitler-cli/internal/flags"
	httpclient "github.com/xochilpili/subtitler-cli/internal/http-client"
	"github.com/xochilpili/subtitler-cli/internal/logger"
)

type Subdivx interface {
	FormatSubtitles(subtitles []Subtitles)
	GetSubtitles(ctx context.Context) []Subtitles
	DownloadSubtitle(ctx context.Context, id int) error
}

type service struct {
	settings   *flags.OptionFlags
	httpClient httpclient.HttpClient
}

var subdivxUrl = "https://subdivx.com/"

func New(settings *flags.OptionFlags) *service {
	httpClient := httpclient.New(settings.Debug)
	return &service{
		settings:   settings,
		httpClient: httpClient,
	}
}

func (s *service) FormatSubtitles(subtitles []Subtitles) {
	tbl := table.NewWriter()
	tbl.SetOutputMirror(os.Stdout)
	tbl.AppendHeader(table.Row{"#", "ID", "Title", "Description"})

	for i, item := range subtitles {
		if item.Title != "" {
			tbl.AppendRow(table.Row{i, item.Id, item.Title, item.Description})
			tbl.AppendSeparator()
			for _, comment := range *item.Comments {
				if comment.Comment != "" {
					tbl.AppendRow(table.Row{"", "", comment.Nick, comment.Comment})
				}
			}
			tbl.AppendSeparator()
		}
	}
	tbl.SetStyle(s.settings.Style)
	tbl.SetAllowedRowLength(300)
	tbl.Render()
}

func (s *service) FormatDownloadedFiles(files []*string) {
	tbl := table.NewWriter()
	tbl.SetOutputMirror(os.Stdout)
	tbl.AppendHeader(table.Row{"#", "File"})
	for i, item := range files {
		tbl.AppendSeparator()
		tbl.AppendRow(table.Row{i, *item})
		tbl.AppendSeparator()
	}
	tbl.AppendFooter(table.Row{"Total Uncompressed:", len(files)})
	tbl.SetStyle(s.settings.Style)
	tbl.Render()
}

func (s *service) getSearchToken(ctx context.Context) Token {
	var target Token

	cookie, err := s.httpClient.Get(ctx, subdivxUrl+"/inc/gt.php?gt=1", &target)
	if err != nil {
		panic(fmt.Errorf("error getting token: %v", err))
	}
	target.Cookie = cookie
	return target
}

func (s *service) GetSubtitles(ctx context.Context) []Subtitles {
	token := s.getSearchToken(ctx)
	payload := &SubdivxSubPayload{
		Tabla:   "resultados",
		Filtros: "",
		Buscar:  s.settings.Title,
		Token:   token.Token,
	}
	cookie := strings.Split(token.Cookie, ";")[0]
	var target SubdivxResponse[SubData]
	response, err := postRequest(ctx, subdivxUrl+"/inc/ajax.php", payload, &target, s.httpClient, cookie)
	if err != nil {
		panic(fmt.Errorf("error getting subtitles %v", err))
	}

	if s.settings.Debug {
		logger.Debug("%v: \n%v", "GetSubtitles.PostRequest response", response)
	}

	var waitGroup sync.WaitGroup
	var subtitles []Subtitles
	subtitlesChan := make(chan Subtitles, len(response.Data))
	reg := regexp.MustCompile("\n|\r\n")
	for _, item := range response.Data {
		waitGroup.Add(1)
		stripTags := bluemonday.StripTagsPolicy()
		title := reg.ReplaceAllString(stripTags.Sanitize(item.Title), " ")
		desc := reg.ReplaceAllString(stripTags.Sanitize(item.Description), " ")
		subtitle := Subtitles{
			Id:          item.Id,
			Title:       s.HighlightString(title),
			Description: s.HighlightString(desc),
			Cds:         item.Cds,
		}

		go s.GetComments(ctx, &subtitle, &waitGroup, subtitlesChan, cookie)
	}
	waitGroup.Wait()
	close(subtitlesChan)

	for item := range subtitlesChan {
		subtitles = append(subtitles, item)
	}
	return subtitles
}

func (s *service) GetComments(ctx context.Context, subtitle *Subtitles, wg *sync.WaitGroup, c chan Subtitles, cookie string) {
	defer wg.Done()
	payload := SubdivxCommentPayload{
		GetComments: strconv.Itoa(int(subtitle.Id)),
	}
	var target SubdivxResponse[SubComments]
	response, err := postRequest(ctx, subdivxUrl+"/inc/ajax.php", &payload, &target, s.httpClient, cookie)
	if err != nil {
		panic(fmt.Errorf("unable to fetch comments for: %d, error: %v", subtitle.Id, err))
	}
	if s.settings.Debug {
		logger.Debug("%v:\n%v", "GetComments.respose", response.Data)
	}

	var comments []SubComments
	stripTags := bluemonday.StripTagsPolicy()
	reg := regexp.MustCompile("\n|\r\n")
	for _, comment := range response.Data {
		// validate if comment is empty
		if comment.Comment != "" {
			desc := reg.ReplaceAllString(stripTags.Sanitize(comment.Comment), " ")
			nick := reg.ReplaceAllString(stripTags.Sanitize(comment.Nick), " ")
			comments = append(comments, SubComments{
				Id:      comment.Id,
				Comment: s.HighlightString(desc),
				Nick:    nick,
				Date:    comment.Date,
			})
		}
	}
	subtitle.Comments = &comments
	c <- *subtitle
}

func (s *service) DownloadSubtitle(ctx context.Context, id int) error {
	subtitleId := strconv.Itoa(int(id))
	logger.Info("%s from: %v", "downloading file", subdivxUrl+"/descargar.php?id="+subtitleId)
	downloadedFile, err := s.httpClient.DownloadFile(ctx, subdivxUrl+"/descargar.php?id="+subtitleId, s.settings.DownloadPath+"/"+subtitleId)
	if err != nil {
		return err
	}

	logger.Info("%s: %v", "downloaded file", downloadedFile)

	// Process downloaded files and clean (which means remove source compressed file)
	archive := file.New(downloadedFile)
	subtitleFles, err := archive.ProcessSubtitles(s.settings.DownloadPath, true)
	if err != nil {
		panic(fmt.Errorf("error while processing downloaded files %v", err))
	}
	s.FormatDownloadedFiles(subtitleFles)

	return nil
}

func (s *service) HighlightString(input string) string {
	var re *regexp.Regexp
	if len(s.settings.Releases) < 1 {
		re = regexp.MustCompile(`(?mi)fgt|evo|yts|yts?\.mx|yifi|yify|MkvCage|NoMeRcY|STRiFE|SiGMA|LucidTV|CHD|sujaidr|SAPHiRE|LEGI0N|hd4u|rarbg|ViSiON|ETRG|JYK|iFT|anoXmous|MkvCage|Ganool|TGx|klaxxon|icebane|greenbud1969|flawl3ss|metcon|proper|ntb|cm8|tbs|sva|avs|mtb|ion10|sauron|phoenix|minx|mvgroup|amiable|sadece|gooz|lite|killers|tbs|PHOENiX|memento|done|ExKinoRay|acool|starz|convoy|playnow|RedBlade|ntg|cmrg|cm|2hd|fty|haggis|Joy|dimension|0tv|fxg|kat|artsubs|horizon|axxo|diamond|asteroids|rarbg|unit3d|afg|xlf|pulsar|bamboozle|ebp|trump|bulit|pahe|lol|tjhd|DeeJayAhmed|DeeJahAhmed|HEVC|anoxmous|galaxy|aoc|flux|roen|silence|CiNEFiLE|wrd|rico|huzzah|RiSEHD`)
	} else {
		re = regexp.MustCompile("(?mi)" + strings.Join(s.settings.Releases, "|"))
	}

	matches := re.FindAllString(input, -1)
	hightlight := color.New(color.FgHiYellow, color.BgHiBlack).SprintFunc()
	words := strings.Fields(input)
	for _, match := range matches {
		reg := regexp.MustCompile("(?mi)" + match)
		for i, w := range words {
			if reg.MatchString(w) {
				words[i] = reg.ReplaceAllString(w, hightlight(match))
			}
		}
	}
	return strings.Join(words, " ")

}

func postRequest[T any](ctx context.Context, endpoint string, payload interface{}, target T, httpClient httpclient.HttpClient, cookie string) (T, error) {
	data := url.Values{}
	switch p := payload.(type) {
	case *SubdivxSubPayload:
		data.Add("tabla", p.Tabla)
		data.Add("filtros", p.Filtros)
		data.Add("buscar393", p.Buscar)
		data.Add("token", p.Token)
	case *SubdivxCommentPayload:
		data.Add("getComentarios", p.GetComments)
	default:
		panic(fmt.Errorf("payload interface not supported"))
	}

	err := httpClient.Post(ctx, endpoint, strings.NewReader(data.Encode()), &target, "form", cookie)
	if err != nil {
		return target, err
	}

	return target, nil
}
