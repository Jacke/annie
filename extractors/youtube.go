package extractors

import (
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"strings"

	"github.com/iawia002/annie/config"
	"github.com/iawia002/annie/downloader"
	"github.com/iawia002/annie/request"
	"github.com/iawia002/annie/utils"
)

type args struct {
	Title  string `json:"title"`
	Stream string `json:"url_encoded_fmt_stream_map"`
}

type assets struct {
	JS string `json:"js"`
}

type youtubeData struct {
	Args   args   `json:"args"`
	Assets assets `json:"assets"`
}

var tokensCache = make(map[string][]string)

func getSig(sig, js string) string {
	url:= fmt.Sprintf("https://www.youtube.com%s", js)
	tokens, ok := tokensCache[url]
	if !ok {
		tokens = getSigTokens(request.Get(url))
		tokensCache[url] = tokens
	}
	return decipherTokens(tokens, sig)
}

// Youtube download function
func Youtube(uri string) {
	if !config.Playlist {
		youtubeDownload(uri)
		return
	}
	listID := utils.MatchOneOf(uri, `(list|p)=([^/&]+)`)[2]
	if listID == "" {
		log.Fatal("Can't get list ID from URL")
	}
	html := request.Get("https://www.youtube.com/playlist?list=" + listID)
	// "videoId":"OQxX8zgyzuM","thumbnail"
	videoIDs := utils.MatchAll(html, `"videoId":"([^,]+?)","thumbnail"`)
	for _, videoID := range videoIDs {
		u := fmt.Sprintf(
			"https://www.youtube.com/watch?v=%s&list=%s", videoID[1], listID,
		)
		youtubeDownload(u)
	}
}

func youtubeDownload(uri string) downloader.VideoData {
	vid := utils.MatchOneOf(
		uri,
		`watch\?v=([^/&]+)`,
		`youtu\.be/([^?/]+)`,
		`embed/([^/?]+)`,
		`v/([^/?]+)`,
	)
	if vid == nil {
		log.Fatal("Can't find vid")
	}
	videoURL := fmt.Sprintf(
		"https://www.youtube.com/watch?v=%s&gl=US&hl=en&has_verified=1&bpctr=9999999999",
		vid[1],
	)
	html := request.Get(videoURL)
	ytplayer := utils.MatchOneOf(html, `;ytplayer\.config\s*=\s*({.+?});`)[1]
	var youtube youtubeData
	json.Unmarshal([]byte(ytplayer), &youtube)
	title := youtube.Args.Title
	streams := strings.Split(youtube.Args.Stream, ",")

	format := map[string]downloader.FormatData{}
	for _, s := range streams {
		stream, _ := url.ParseQuery(s)
		quality := stream.Get("quality")
		ext := utils.MatchOneOf(stream.Get("type"), `video/(\w+);`)[1]
		streamURL := stream.Get("url")
		itag := stream.Get("itag")
		var realURL string
		if strings.Contains(streamURL, "signature=") {
			// URL itself already has a signature parameter
			realURL = streamURL
		} else {
			// URL has no signature parameter
			sig := stream.Get("sig")
			if sig == "" {
				// Signature need decrypt
				sig = getSig(stream.Get("s"), youtube.Assets.JS)
			}
			realURL = fmt.Sprintf("%s&signature=%s", streamURL, sig)
		}
		size := request.Size(realURL, uri)
		urlData := downloader.URLData{
			URL:  realURL,
			Size: size,
			Ext:  ext,
		}
		format[itag] = downloader.FormatData{
			URLs:    []downloader.URLData{urlData},
			Size:    size,
			Quality: quality,
		}
	}
	stream, _ := url.ParseQuery(streams[0]) // Best quality
	format["default"] = format[stream.Get("itag")]
	delete(format, stream.Get("itag"))

	extractedData := downloader.VideoData{
		Site:    "YouTube youtube.com",
		Title:   utils.FileName(title),
		Type:    "video",
		Formats: format,
	}
	extractedData.Download(uri)
	return extractedData
}
