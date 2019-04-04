package whatweb

import (
	"encoding/json"
	"fmt"
	"github.com/PuerkitoBio/goquery"
	"github.com/prometheus/common/log"
	"io/ioutil"
	"regexp"
	"strconv"
	"strings"
)

/*
对 https://github.com/unstppbl/gowap/blob/master/gowap.go 进行修改

传入以下数据即可进行分析

( url, 响应头[list], 网页内容, js返回内容 )

TODO:
	jsret
*/

type HttpData struct {
	Url		string
	Headers	map[string][]string
	Html	string
	Jsret	string
}

type analyzeData struct {
	scripts []string
	cookies map[string]string
}

type temp struct {
	Apps       map[string]*json.RawMessage `json:"apps"`
	Categories map[string]*json.RawMessage `json:"categories"`
}

type application struct {
	Name       string   `json:"name,ompitempty"`
	Version    string   `json:"version"`
	Categories []string `json:"categories,omitempty"`

	Cats     []int                  `json:"cats,omitempty"`
	Cookies  interface{}            `json:"cookies,omitempty"`
	Js       interface{}            `json:"js,omitempty"`
	Headers  interface{}            `json:"headers,omitempty"`
	HTML     interface{}            `json:"html,omitempty"`
	Excludes interface{}            `json:"excludes,omitempty"`
	Implies  interface{}            `json:"implies,omitempty"`
	Meta     map[string]interface{} `json:"meta,omitempty"`
	Scripts  interface{}            `json:"script,omitempty"`
	URL      string                 `json:"url,omitempty"`
	Website  string                 `json:"website,omitempty"`
}

type category struct {
	Name     string `json:"name,omitempty"`
	Priority int    `json:"priority,omitempty"`
}

// Wappalyzer implements analyze method as original wappalyzer does
type Wappalyzer struct {
	HttpData  *HttpData
	Apps       map[string]*application
	Categories map[string]*category
	JSON       bool
}

// Init
func Init(appsJSONPath string, JSON bool) (wapp *Wappalyzer, err error) {
	wapp = &Wappalyzer{}
	//wapp.Transport = &http.Transport{
	//	TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	//}
	appsFile, err := ioutil.ReadFile(appsJSONPath)
	if err != nil {
		log.Errorf("Couldn't open file at %s\n", appsJSONPath)
		return nil, err
	}

	temporary := &temp{}
	err = json.Unmarshal(appsFile, &temporary)
	if err != nil {
		log.Errorf("Couldn't unmarshal apps.json file: %s\n", err)
		return nil, err
	}

	wapp.Apps = make(map[string]*application)
	wapp.Categories = make(map[string]*category)

	for k, v := range temporary.Categories {
		catg := &category{}
		if err = json.Unmarshal(*v, catg); err != nil {
			log.Errorf("[!] Couldn't unmarshal Categories: %s\n", err)
			return nil, err
		}
		wapp.Categories[k] = catg
	}

	for k, v := range temporary.Apps {
		app := &application{}
		app.Name = k
		if err = json.Unmarshal(*v, app); err != nil {
			log.Errorf("Couldn't unmarshal Apps: %s\n", err)
			return nil, err
		}
		parseCategories(app, &wapp.Categories)
		wapp.Apps[k] = app
	}
	wapp.JSON = JSON

	return wapp, nil
}

type resultApp struct {
	Name       string   `json:"name,ompitempty"`
	Version    string   `json:"version"`
	Categories []string `json:"categories,omitempty"`
	excludes   interface{}
	implies    interface{}
}

func (wapp *Wappalyzer) ConvHeader(headers string) (map[string][]string) {
	head := make(map[string][]string)

	tmp := strings.Split(strings.TrimRight(headers, "\n"), "\n")
	for _, v := range tmp {
		if strings.HasPrefix(strings.ToLower(v), "http/1.") {
			continue
		}
		splitStr := strings.Split(v, ":")
		header_key := strings.ToLower(strings.Replace(splitStr[0], "_", "-", -1))
		header_val := strings.TrimSpace(strings.Join(splitStr[1:],""))

		head[header_key] = append(head[header_key], header_val)
	}

	return head
}

func (wapp *Wappalyzer) Analyze(httpdata *HttpData) (result interface{}, err error) {
	analyzeData := &analyzeData{}
	detectedApplications := make(map[string]*resultApp)

	// analyze html script src
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(httpdata.Html))
	if err != nil {
		log.Fatal(err)
	}

	doc.Find("script").Each(func(i int, s *goquery.Selection) {
		url, err := s.Attr("src")
		if err == true {
			analyzeData.scripts = append(analyzeData.scripts, url)
		}
	})

	/*
		analyze headers cookie

		two set-cookie?
	 */
	analyzeData.cookies = make(map[string]string)
	for _, cookie := range httpdata.Headers["set-cookie"] {
		keyValues := strings.Split(cookie, ";")
		for _, keyValueString := range keyValues {
			keyValueSlice := strings.Split(keyValueString, "=")
			if len(keyValueSlice) > 1 {
				key, value := keyValueSlice[0], keyValueSlice[1]
				analyzeData.cookies[key] = value
			}
		}
	}

	for _, app := range wapp.Apps {
		analyzeURL(app, httpdata.Url, &detectedApplications)
		if app.HTML != nil {
			analyzeHTML(app, httpdata.Html, &detectedApplications)
		}
		if app.Headers != nil {
			analyzeHeaders(app, httpdata.Headers, &detectedApplications)
		}
		if app.Cookies != nil {
			analyzeCookies(app, analyzeData.cookies, &detectedApplications)
		}
		if app.Scripts != nil {
			analyzeScripts(app, analyzeData.scripts, &detectedApplications)
		}
	}

	for _, app := range detectedApplications {
		if app.excludes != nil {
			resolveExcludes(&detectedApplications, app.excludes)
		}
		if app.implies != nil {
			resolveImplies(&wapp.Apps, &detectedApplications, app.implies)
		}
	}

	res := []map[string]interface{}{}
	for _, app := range detectedApplications {
		// log.Printf("URL: %-25s DETECTED APP: %-20s VERSION: %-8s CATEGORIES: %v", url, app.Name, app.Version, app.Categories)
		res = append(res, map[string]interface{}{"name": app.Name, "version": app.Version, "categories": app.Categories})
	}
	if wapp.JSON {
		j, err := json.Marshal(res)
		if err != nil {
			return nil, err
		}
		return string(j), nil
	}

	return res, nil
}

func parseImpliesExcludes(value interface{}) (array []string) {
	switch item := value.(type) {
	case string:
		array = append(array, item)
	case []string:
		return item
	}
	return array
}

func resolveExcludes(detected *map[string]*resultApp, value interface{}) {
	excludedApps := parseImpliesExcludes(value)
	for _, excluded := range excludedApps {
		delete(*detected, excluded)
	}
}

func resolveImplies(apps *map[string]*application, detected *map[string]*resultApp, value interface{}) {
	impliedApps := parseImpliesExcludes(value)
	for _, implied := range impliedApps {
		app, ok := (*apps)[implied]
		if _, ok2 := (*detected)[implied]; ok && !ok2 {
			resApp := &resultApp{app.Name, app.Version, app.Categories, app.Excludes, app.Implies}
			(*detected)[implied] = resApp
			if app.Implies != nil {
				resolveImplies(apps, detected, app.Implies)
			}
		}
	}
}

func parseCategories(app *application, categoriesCatalog *map[string]*category) {
	for _, categoryID := range app.Cats {
		app.Categories = append(app.Categories, (*categoriesCatalog)[strconv.Itoa(categoryID)].Name)
	}
}

type pattern struct {
	str        string
	regex      *regexp.Regexp
	version    string
	confidence string
}

func parsePatterns(patterns interface{}) (result map[string][]*pattern) {
	parsed := make(map[string][]string)
	switch ptrn := patterns.(type) {
	case string:
		parsed["main"] = append(parsed["main"], ptrn)
	case map[string]interface{}:
		for k, v := range ptrn {
			parsed[k] = append(parsed[k], v.(string))
		}
	case []interface{}:
		var slice []string
		for _, v := range ptrn {
			slice = append(slice, v.(string))
		}
		parsed["main"] = slice
	default:
		log.Errorf("Unkown type in parsePatterns: %T\n", ptrn)
	}
	result = make(map[string][]*pattern)
	for k, v := range parsed {
		for _, str := range v {
			appPattern := &pattern{}
			slice := strings.Split(str, "\\;")
			for i, item := range slice {
				if item == "" {
					continue
				}
				if i > 0 {
					additional := strings.Split(item, ":")
					if len(additional) > 1 {
						if additional[0] == "version" {
							appPattern.version = additional[1]
						} else {
							appPattern.confidence = additional[1]
						}
					}
				} else {
					appPattern.str = item
					first := strings.Replace(item, `\/`, `/`, -1)
					second := strings.Replace(first, `\\`, `\`, -1)
					reg, err := regexp.Compile(fmt.Sprintf("%s%s", "(?i)", strings.Replace(second, `/`, `\/`, -1)))
					if err == nil {
						appPattern.regex = reg
					}
				}
			}
			result[k] = append(result[k], appPattern)
		}
	}
	return result
}

func analyzeURL(app *application, url string, detectedApplications *map[string]*resultApp) {
	patterns := parsePatterns(app.URL)
	for _, v := range patterns {
		for _, pattrn := range v {
			if pattrn.regex != nil && pattrn.regex.Match([]byte(url)) {
				if _, ok := (*detectedApplications)[app.Name]; !ok {
					resApp := &resultApp{app.Name, app.Version, app.Categories, app.Excludes, app.Implies}
					(*detectedApplications)[resApp.Name] = resApp
					detectVersion(resApp, pattrn, &url)
				}
			}
		}
	}
}

func detectVersion(app *resultApp, pattrn *pattern, value *string) {
	versions := make(map[string]interface{})
	version := pattrn.version
	if slices := pattrn.regex.FindAllStringSubmatch(*value, -1); slices != nil {
		for _, slice := range slices {
			for i, match := range slice {
				reg, _ := regexp.Compile(fmt.Sprintf("%s%d%s", "\\\\", i, "\\?([^:]+):(.*)$"))
				ternary := reg.FindAll([]byte(version), -1)
				if ternary != nil && len(ternary) == 3 {
					version = strings.Replace(version, string(ternary[0]), string(ternary[1]), -1)
				}
				reg2, _ := regexp.Compile(fmt.Sprintf("%s%d", "\\\\", i))
				version = reg2.ReplaceAllString(version, match)
			}
		}
		if _, ok := versions[version]; ok != true && version != "" {
			versions[version] = struct{}{}
		}
		if len(versions) != 0 {
			for ver := range versions {
				if ver > app.Version {
					app.Version = ver
				}
			}
		}
	}
}

/*
	fix some bug:
	1. 如果regex为空的话, 就看headers名是否存在了
*/
func analyzeHeaders(app *application, headers map[string][]string, detectedApplications *map[string]*resultApp) {
	patterns := parsePatterns(app.Headers)
	for headerName, v := range patterns {
		headerNameLowerCase := strings.ToLower(headerName)

		for _, pattrn := range v {
			headersSlice, ok := headers[headerNameLowerCase]

			if !ok {
				continue
			}

			if ok && pattrn.regex  == nil {
				resApp := &resultApp{app.Name, app.Version, app.Categories, app.Excludes, app.Implies}
				(*detectedApplications)[resApp.Name] = resApp
			}

			if ok {
				for _, header := range headersSlice {
					if pattrn.regex != nil && pattrn.regex.Match([]byte(header)) {
						if _, ok := (*detectedApplications)[app.Name]; !ok {
							resApp := &resultApp{app.Name, app.Version, app.Categories, app.Excludes, app.Implies}
							(*detectedApplications)[resApp.Name] = resApp
							detectVersion(resApp, pattrn, &header)
						}
					}
				}
			}
		}
	}
}

func analyzeHTML(app *application, html string, detectedApplications *map[string]*resultApp) {
	patterns := parsePatterns(app.HTML)
	for _, v := range patterns {
		for _, pattrn := range v {
			if pattrn.regex != nil && pattrn.regex.Match([]byte(html)) {
				if _, ok := (*detectedApplications)[app.Name]; !ok {
					resApp := &resultApp{app.Name, app.Version, app.Categories, app.Excludes, app.Implies}
					(*detectedApplications)[resApp.Name] = resApp
					detectVersion(resApp, pattrn, &html)
				}
			}
		}
	}
}

func analyzeScripts(app *application, scripts []string, detectedApplications *map[string]*resultApp) {
	patterns := parsePatterns(app.Scripts)
	for _, v := range patterns {
		for _, pattrn := range v {
			if pattrn.regex != nil {
				for _, script := range scripts {
					if pattrn.regex.Match([]byte(script)) {
						if _, ok := (*detectedApplications)[app.Name]; !ok {
							resApp := &resultApp{app.Name, app.Version, app.Categories, app.Excludes, app.Implies}
							(*detectedApplications)[resApp.Name] = resApp
							detectVersion(resApp, pattrn, &script)
						}
					}
				}
			}
		}
	}
}

/*
	fix some bug:
	1. 如果regex为空的话, 就看session名是否存在了
*/
func analyzeCookies(app *application, cookies map[string]string, detectedApplications *map[string]*resultApp) {
	patterns := parsePatterns(app.Cookies)
	for cookieName, v := range patterns {
		cookieNameLowerCase := strings.ToLower(cookieName)
		for _, pattrn := range v {
			cookie, ok := cookies[cookieNameLowerCase];

			if !ok {
				continue
			}

			//fmt.Println(cookie, cookieName, pattrn)

			if ok && pattrn.regex  == nil {
				if _, ok := (*detectedApplications)[app.Name]; !ok {
					resApp := &resultApp{app.Name, app.Version, app.Categories, app.Excludes, app.Implies}
					(*detectedApplications)[resApp.Name] = resApp
				}
			}

			if ok && pattrn.regex != nil && pattrn.regex.MatchString(cookie) {
				if _, ok := (*detectedApplications)[app.Name]; !ok {
					resApp := &resultApp{app.Name, app.Version, app.Categories, app.Excludes, app.Implies}
					(*detectedApplications)[resApp.Name] = resApp
					detectVersion(resApp, pattrn, &cookie)
				}
			}
		}
	}
}

