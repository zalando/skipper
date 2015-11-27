package innkeeper

import (
    "testing"
    "net/http"
    "net/http/httptest"
    // "net/url"
    // "github.com/zalando/skipper/eskip"
)

func testServer() *httptest.Server {
    return httptest.NewServer(http.HandlerFunc(func (w http.ResponseWriter, r *http.Request) {
        w.Write([]byte(`[{
            "name": "THE_NEW_ROUTE",
            "activateAt": "2015-11-19T15:48:49.867",
            "id": 5,
            "createdAt": "2015-11-19T15:47:49.867",
            "route": {
                "matcher": {
                    "pathMatcher": {
                        "match": "\/",
                        "type": "STRICT"
                    },
                    "headerMatchers": []
                },
                "filters": []
            }
        }]`))
    }))
}

func TestUpdate(t *testing.T) {
    // server := testServer()
    // su, err := url.Parse(server.URL)
    // if err != nil {
    //     t.Error(err)
    // }
    // su.Path = ""

    // client, err := New(Options{Address: su.String()})
    // if err != nil {
    //     t.Error(err)
    // }

    // routes, err := client.LoadAll()
    // if err != nil {
    //     t.Error(err)
    // }

    // t.Error(eskip.String(routes...))
}
