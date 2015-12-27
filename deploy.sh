go generate
go build .
rsync -azv {*.go,*.proto,static,ficrecommend} raven.fn.lc:/home/rice/fic2
#ssh rice@raven.fn.lc -C "export GOPATH=/home/rice/go && cd fic2 && go generate && go build"
