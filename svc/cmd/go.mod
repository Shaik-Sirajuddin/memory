module github.com/Shaik-Sirajuddin/memory/svc/cmd

go 1.26.1

require (
	github.com/Shaik-Sirajuddin/memory/mcp v0.0.0-00010101000000-000000000000
	github.com/Shaik-Sirajuddin/memory/pkg/log v0.0.0
	github.com/Shaik-Sirajuddin/memory/svc/hook-operator v0.0.0
	github.com/Shaik-Sirajuddin/memory/svc/ptydaemon v0.0.0
)

require (
	github.com/Shaik-Sirajuddin/memory v0.0.0 // indirect
	github.com/adrg/xdg v0.5.3 // indirect
	github.com/creack/pty v1.1.24 // indirect
	github.com/dustin/go-humanize v1.0.1 // indirect
	github.com/go-viper/mapstructure/v2 v2.5.0 // indirect
	github.com/google/jsonschema-go v0.4.2 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/knadh/koanf/maps v0.1.2 // indirect
	github.com/knadh/koanf/parsers/json v1.0.0 // indirect
	github.com/knadh/koanf/providers/rawbytes v1.0.0 // indirect
	github.com/knadh/koanf/v2 v2.3.4 // indirect
	github.com/mark3labs/mcp-go v0.54.0 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/mitchellh/copystructure v1.2.0 // indirect
	github.com/mitchellh/reflectwalk v1.0.2 // indirect
	github.com/ncruces/go-strftime v1.0.0 // indirect
	github.com/remyoudompheng/bigfft v0.0.0-20230129092748-24d4a6f8daec // indirect
	github.com/santhosh-tekuri/jsonschema/v6 v6.0.2 // indirect
	github.com/spf13/cast v1.7.1 // indirect
	github.com/yosida95/uritemplate/v3 v3.0.2 // indirect
	golang.org/x/sys v0.43.0 // indirect
	golang.org/x/text v0.14.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
	modernc.org/libc v1.72.0 // indirect
	modernc.org/mathutil v1.7.1 // indirect
	modernc.org/memory v1.11.0 // indirect
	modernc.org/sqlite v1.50.0 // indirect
)

replace (
	github.com/Shaik-Sirajuddin/memory => ../../omni
	github.com/Shaik-Sirajuddin/memory/mcp => ../../tunnel_mcp
	github.com/Shaik-Sirajuddin/memory/pkg/log => ../../pkg/log
	github.com/Shaik-Sirajuddin/memory/svc/hook-operator => ../hook-operator
	github.com/Shaik-Sirajuddin/memory/svc/ptydaemon => ../ptydaemon
)
