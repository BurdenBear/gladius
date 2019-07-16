CUR_DIR=$(cd "$(dirname "${BASH_SOURCE-$0}")"; pwd)
cd $CUR_DIR
go build --buildmode=plugin -o ../strategy.so main.go