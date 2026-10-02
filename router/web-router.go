package router

import (
	"bytes"
	"embed"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/controller"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/setting/console_setting"
	"github.com/gin-contrib/gzip"
	"github.com/gin-contrib/static"
	"github.com/gin-gonic/gin"
	"golang.org/x/net/html"
)

// WebAssets holds the embedded dashboard frontend assets.
type WebAssets struct {
	BuildFS   embed.FS
	IndexPage []byte
}

func setHTMLAttribute(node *html.Node, key, value string) {
	for i := range node.Attr {
		if node.Attr[i].Key == key {
			node.Attr[i].Val = value
			return
		}
	}
	node.Attr = append(node.Attr, html.Attribute{Key: key, Val: value})
}

func findHTMLElement(root *html.Node, element, attribute, value string) *html.Node {
	var found *html.Node
	var walk func(*html.Node)
	walk = func(node *html.Node) {
		if found != nil {
			return
		}
		if node.Type == html.ElementNode && node.Data == element {
			if attribute == "" {
				found = node
				return
			}
			for _, attr := range node.Attr {
				if attr.Key == attribute && attr.Val == value {
					found = node
					return
				}
			}
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(root)
	return found
}

func upsertMeta(head *html.Node, attribute, name, content string) {
	node := findHTMLElement(head, "meta", attribute, name)
	if node == nil {
		node = &html.Node{Type: html.ElementNode, Data: "meta"}
		setHTMLAttribute(node, attribute, name)
		head.AppendChild(node)
	}
	setHTMLAttribute(node, "content", content)
}

func setElementText(node *html.Node, content string) {
	for child := node.FirstChild; child != nil; {
		next := child.NextSibling
		node.RemoveChild(child)
		child = next
	}
	node.AppendChild(&html.Node{Type: html.TextNode, Data: content})
}

func upsertTitle(head *html.Node, title string) {
	node := findHTMLElement(head, "title", "", "")
	if node == nil {
		node = &html.Node{Type: html.ElementNode, Data: "title"}
		head.AppendChild(node)
	}
	setElementText(node, title)
}

// renderSPAIndex rewrites the public metadata on the SPA shell.
//
// The tab icon is deliberately left alone. The configured logo is normally a
// wide wordmark, and a browser scales whatever it is given to fill the square
// tab slot, so using it here squashed the mark sideways. The packaged square
// icon is already the site's mark, undistorted, so it stays as the href.
func renderSPAIndex(indexPage []byte, systemName string, meta console_setting.SPAMetaSetting) ([]byte, error) {
	document, err := html.Parse(bytes.NewReader(indexPage))
	if err != nil {
		return nil, err
	}
	head := findHTMLElement(document, "head", "", "")
	if head == nil {
		return nil, nil
	}

	if systemName != "" {
		upsertTitle(head, systemName)
		upsertMeta(head, "name", "title", systemName)
		upsertMeta(head, "property", "og:title", systemName)
	}
	upsertMeta(head, "name", "description", meta.Description)
	upsertMeta(head, "property", "og:type", meta.OGType)
	upsertMeta(head, "property", "og:description", meta.OGDescription)

	var output bytes.Buffer
	if err = html.Render(&output, document); err != nil {
		return nil, err
	}
	return output.Bytes(), nil
}

func SetWebRouter(router *gin.Engine, assets WebAssets, pluginDispatcher gin.HandlerFunc) {
	frontendFS := common.EmbedFolder(assets.BuildFS, "web/dist")

	router.NoRoute(
		pluginDispatcher,
		middleware.RouteTag("web"),
		// Fonts are already compressed; gzipping them costs CPU and saves no
		// bytes. The library's default list only covers raster images.
		gzip.Gzip(gzip.DefaultCompression, gzip.WithExcludedExtensions([]string{
			".png", ".gif", ".jpeg", ".jpg",
			".woff", ".woff2", ".ttf", ".otf", ".eot",
			".ico", ".svg", ".zip", ".gz", ".br", ".mp4", ".webp",
		})),
		middleware.AccessTokenAudit(),
		middleware.GlobalWebRateLimit(),
		func(c *gin.Context) {
			if strings.HasPrefix(c.Request.RequestURI, "/v1") || strings.HasPrefix(c.Request.RequestURI, "/api") || strings.HasPrefix(c.Request.RequestURI, "/assets") {
				c.Header("Cache-Control", "no-store, no-cache, must-revalidate, private, max-age=0")
				controller.RelayNotFound(c)
				return
			}
			if frontendFS.Exists("/", c.Request.URL.Path) {
				middleware.Cache()(c)
				static.Serve("/", frontendFS)(c)
				return
			}
			middleware.Cache()(c)
			c.Header("Cache-Control", "no-cache")
			common.OptionMapRWMutex.RLock()
			systemName := common.SystemName
			meta := console_setting.GetSPAMetaSetting()
			common.OptionMapRWMutex.RUnlock()
			indexPage, err := renderSPAIndex(assets.IndexPage, systemName, meta)
			if err != nil || indexPage == nil {
				indexPage = assets.IndexPage
			}
			c.Data(http.StatusOK, "text/html; charset=utf-8", indexPage)
		},
	)
}
