go generate
rsync -azv {*.go,static} raven.fn.lc:/home/rice/fic2
ssh rice@raven.fn.lc -C "export GOPATH=/home/rice/go && cd fic2 && go build"
