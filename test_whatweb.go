package main

import (
	"fmt"
	"github.com/prometheus/common/log"
	"study/src/module"
)


func main()  {
	wapp, _ := whatweb.Init("src/data/app.json", false)

	httpdata := &whatweb.HttpData{}
	httpdata.Url = "https://cpanel.lets-wedding.com/"
	httpdata.Html = ""

	headers := "HTTP/1.1 307 Temporary Redirect\npragma: no-cache\ncache_control: no-store, no-cache, must-revalidate, post-check=0, pre-check=0\ncontent_length: 0\ndate: Wed, 03 Apr 2019 17:20:57 GMT\nlocation: http://www.google.com.tw\ncontent_type: text/html; charset=UTF-8\nexpires: Thu, 19 Nov 1981 08:52:00 GMT\nset_cookie: ci_session=2e88f40a4f75728b2e106d7be46ce839c67204bb; expires=Wed, 03-Apr-2019 19:20:57 GMT; Max-Age=7200; path=/; HttpOnly\nconnection: keep-alive\nserver: nginx/1.14.1\n"
	httpdata.Headers = wapp.ConvHeader(headers)
	res, err := wapp.Analyze(httpdata)

	fmt.Println(httpdata.Url)
	if err != nil {
		log.Errorln(err)
	}
	fmt.Println(res)
}