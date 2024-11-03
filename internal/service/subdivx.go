package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/fatih/color"
	"github.com/go-resty/resty/v2"
	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/microcosm-cc/bluemonday"
	files "github.com/xochilpili/subtitler-cli/internal/files"
	"github.com/xochilpili/subtitler-cli/internal/flags"
	"github.com/xochilpili/subtitler-cli/internal/logger"
)

type subdivx struct {
	settings   *flags.OptionFlags
	r *resty.Client
}

var baseUrl = "https://subdivx.com/"
var userAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36"

func NewSub(settings *flags.OptionFlags) *subdivx {
	r := resty.New()
	return &subdivx{
		settings:   settings,
		r: r,
	}
}


func (s *subdivx) FormatSubtitles(subtitles []Subtitles) {
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

func (s *subdivx) FormatDownloadedFiles(files []*string) {
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

func (s *subdivx) getVersion(ctx context.Context) (string, error) {
	res, err := s.r.R().SetDebug(s.settings.Debug).SetContext(ctx).Get(baseUrl)
	if err != nil {
		return "", errors.New("error while requesting version")
	}
	re := regexp.MustCompile(`<div[^>]*id="vs"[^>]*>([^<]+)</div>`)
	match := re.FindStringSubmatch(string(res.Body()))
	if len(match) > 1 {
		version := match[1]
		return strings.Trim(strings.Replace(strings.TrimPrefix(version, "v"), ".", "", -1), "\n"), nil
	}
	return "", errors.New("error while parsing version")
}

func (s *subdivx) getToken(ctx context.Context) (*Token, error) {
	var token Token
	res, err := s.r.R().
		SetContext(ctx).
		SetHeaders(map[string]string{"Content-Type": "application/json", "User-Agent": userAgent}).
		SetQueryParam("gt", "1").
		SetDebug(s.settings.Debug).
		Get(baseUrl + "inc/gt.php")

	if err != nil {
		return nil, err
	}

	err = json.Unmarshal(res.Body(), &token)
	if err != nil {
		return nil, err
	}

	return &token, nil
}

func (s *subdivx) GetSubtitles(ctx context.Context) ([]Subtitles, error) {
	version, _ := s.getVersion(ctx)
	token, _ := s.getToken(ctx)
	params := &SubdivxSubPayload{
		Tabla:   "resultados",
		Filtros: "",
		Buscar:  s.settings.Title,
		Token:   token.Token,
	}
	buscaVersion := fmt.Sprintf("buscar%s", version)
	queryParams := map[string]string{
		"tabla":      params.Tabla,
		"filtros":    params.Filtros,
		buscaVersion: params.Buscar,
		"token":      params.Token,
	}
	var result SubdivxResponse[SubData]
	

	s.r.SetRetryCount(10).SetRetryWaitTime(5*time.Second)
	s.r.AddRetryCondition(func(r *resty.Response, _ error) bool {
		errs := json.Unmarshal(r.Body(), &result)
		if errs != nil{
			return false
		}
		ok, err := strconv.Atoi(result.Secho)
		if err != nil{
			return false
		}
		fmt.Printf("retry: %t, data: %+v\n", ok == 0, len(result.Data))
		return ok == 0
	})
	
	

	resp, err := s.r.R().
		SetContext(ctx).
		SetFormData(queryParams).
		SetHeaders(map[string]string{
			"Content-Type": "application/x-www-form-urlencoded",
			"User-Agent":  userAgent,
		}).
		SetDebug(s.settings.Debug).
		Post(baseUrl + "inc/ajax.php")
	if err != nil {
		return nil, err
	}

	err = json.Unmarshal(resp.Body(), &result)
	if err != nil {
		return nil, err
	}

	if s.settings.Debug {
		logger.Debug("%v: \n%v", "GetSubtitles.PostRequest response", string(resp.Body()))
	}

	if(result.Message == "Por favor espera unos segundos antes de realizar otra busqueda."){
		
	}

	var waitGroup sync.WaitGroup
	var subtitles []Subtitles
	subtitlesChan := make(chan Subtitles, len(result.Data))
	reg := regexp.MustCompile("\n|\r\n")
	for _, item := range result.Data {
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

		go s.GetComments(ctx, &subtitle, &waitGroup, subtitlesChan)
	}
	waitGroup.Wait()
	close(subtitlesChan)

	for item := range subtitlesChan {
		subtitles = append(subtitles, item)
	}
	return subtitles, nil
}

func (s *subdivx) GetComments(ctx context.Context, subtitle *Subtitles, wg *sync.WaitGroup, c chan Subtitles) {
	defer wg.Done()
	
	var result SubdivxResponse[SubComments]
	res, err := s.r.R().
		SetHeaders(map[string]string{
			"Content-Type": "application/x-www-form-urlencoded; charset=UTF-8",
			"User-Agent":   userAgent,
		}).
		SetFormData(map[string]string{
			"getComentarios": strconv.Itoa(subtitle.Id),
		}).
		SetDebug(s.settings.Debug).
		Post(baseUrl + "inc/ajax.php")
	if err != nil {
		logger.Error("%v: \n%v", "GetSubtitles.PostRequest response", err.Error())
	}

	err = json.Unmarshal(res.Body(), &result)
	if err != nil {
		logger.Error("%v: \n%v", "error getting comments", err.Error())
	}

	var comments []SubComments
	stripTags := bluemonday.StripTagsPolicy()
	reg := regexp.MustCompile("\n|\r\n")
	for _, comment := range result.Data {
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

func (s *subdivx) DownloadSubtitle(ctx context.Context, subtitleId int) error {
	id := strconv.Itoa(int(subtitleId))
	res, err := s.r.R().
		SetContext(ctx).
		SetHeaders(map[string]string{
			"User-Agent": userAgent,
			"Referer":    baseUrl + "descargar.php",
			"Connection": "keep-alive",
		}).
		SetDebug(s.settings.Debug).
		SetDoNotParseResponse(true).
		SetQueryParam("id", id).
		Get(baseUrl + "descargar.php")
	if err != nil {
		return err
	}

	contentType := res.Header().Get("Content-Type")
	ext := strings.Split(contentType, "/")[1]
	filename := fmt.Sprintf("%d.%s", subtitleId, ext)
	file, err := os.Create(filename)
	if err != nil{
		return err
	}
	_, err = io.Copy(file, res.RawBody())
	if err != nil{
		return err
	}
	logger.Info("%v: \n%s", "downloaded file %s", filename)
	// Process downloaded files and clean (which means remove source compressed file)
	archive := files.New(filename)
	subtitleFles, err := archive.ProcessSubtitles(s.settings.DownloadPath, true)
	if err != nil {
		panic(fmt.Errorf("error while processing downloaded files %v", err))
	}
	s.FormatDownloadedFiles(subtitleFles)

	return nil
}

func (s *subdivx) HighlightString(input string) string {
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
