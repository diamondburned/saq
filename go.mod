module libdb.so/saq

go 1.20

replace github.com/fsnotify/fsnotify => github.com/fsnotify/fsnotify v1.6.1-0.20230120163138-c7cf79d65078

require (
	github.com/diamondburned/ghproxy v0.0.0-20201025235419-194be0dfdd7b
	github.com/fsnotify/fsnotify v1.6.1-0.20230209173220-cb6339e660b4
	github.com/spf13/pflag v1.0.5
	golang.org/x/sync v0.2.0
	libdb.so/hserve v0.0.0-20230404043009-95e112a6e0a5
)

require (
	github.com/pkg/errors v0.9.1 // indirect
	github.com/sabhiram/go-gitignore v0.0.0-20210923224102-525f6e181f06 // indirect
	golang.org/x/sys v0.4.0 // indirect
)
