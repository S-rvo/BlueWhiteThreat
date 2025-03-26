FROM golang:1.24

WORKDIR /app

# On ne copie rien ici — tout sera monté en volume depuis le host

CMD /bin/sh -c "go test ./... && go build -o crawler ./cmd/main.go && ./crawler"
