module github.com/mikecoop83/TivoToDoList

go 1.18

require (
	github.com/mikecoop83/json v0.0.0-00010101000000-000000000000
	github.com/mikecoop83/tivo v0.0.0-00010101000000-000000000000
	google.golang.org/api v0.70.0
	gopkg.in/jpoehls/gophermail.v0 v0.0.0-20160410235621-62941eab772c
)

require (
	cloud.google.com/go/compute v1.5.0 // indirect
	github.com/golang/groupcache v0.0.0-20210331224755-41bb18bfe9da // indirect
	github.com/golang/protobuf v1.5.2 // indirect
	github.com/googleapis/gax-go/v2 v2.1.1 // indirect
	go.opencensus.io v0.23.0 // indirect
	golang.org/x/net v0.0.0-20220225172249-27dd8689420f // indirect
	golang.org/x/oauth2 v0.0.0-20220223155221-ee480838109b // indirect
	golang.org/x/text v0.3.7 // indirect
	google.golang.org/appengine v1.6.7 // indirect
	google.golang.org/genproto v0.0.0-20220222213610-43724f9ea8cf // indirect
	google.golang.org/grpc v1.44.0 // indirect
	google.golang.org/protobuf v1.27.1 // indirect
)

require (
	github.com/sloonz/go-qprintable v0.0.0-20210417175225-715103f9e6eb // indirect
	golang.org/x/sys v0.0.0-20220224120231-95c6836cb0e7 // indirect
)

replace github.com/mikecoop83/tivo => ../go-tivo

replace github.com/mikecoop83/json => ../go-json
