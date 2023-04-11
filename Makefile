.PHONY: table2struct
lint:
	go run cmd/cli.go -file model.go -dsn 'user:passworld@tcp(db-master-service.default.svc.cluster.local:3306)/xiaomiquan?charset=utf8' -table 'user' -prefix 'mi_'
