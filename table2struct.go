package converter

import (
	"database/sql"
	"errors"
	"fmt"
	_ "github.com/go-sql-driver/mysql"
	"log"
	"os"
	"os/exec"
	"strings"
)

// map for converting mysql type to golang types
var typeForMysqlToGo = map[string]string{
	"int":                "int64",
	"integer":            "int64",
	"tinyint":            "int64",
	"smallint":           "int64",
	"mediumint":          "int64",
	"bigint":             "int64",
	"int unsigned":       "int64",
	"integer unsigned":   "int64",
	"tinyint unsigned":   "int64",
	"smallint unsigned":  "int64",
	"mediumint unsigned": "int64",
	"bigint unsigned":    "int64",
	"bit":                "int64",
	"bool":               "bool",
	"enum":               "string",
	"set":                "string",
	"varchar":            "string",
	"char":               "string",
	"tinytext":           "string",
	"mediumtext":         "string",
	"text":               "string",
	"longtext":           "string",
	"blob":               "string",
	"tinyblob":           "string",
	"mediumblob":         "string",
	"longblob":           "string",
	"date":               "time.Time", // time.Time or string
	"datetime":           "time.Time", // time.Time or string
	"timestamp":          "time.Time", // time.Time or string
	"time":               "time.Time", // time.Time or string
	"float":              "float64",
	"double":             "float64",
	"decimal":            "float64",
	"binary":             "string",
	"varbinary":          "string",
}

type Table2Struct struct {
	dsn            string
	savePath       string
	db             *sql.DB
	table          string
	prefix         string
	config         *T2tConfig
	err            error
	realNameMethod string
	enableJsonTag  bool   // 是否添加json的tag, 默认不添加
	packageName    string // 生成struct的包名(默认为空的话, 则取名为: package model)
	tagKey         string // tag字段的key值,默认是orm
	dateToTime     bool   // 是否将 date相关字段转换为 time.Time,默认否
}

type T2tConfig struct {
	StructNameToHump bool // 结构体名称是否转为驼峰式，默认为false
	RmTagIfUcFirsted bool // 如果字段首字母本来就是大写, 就不添加tag, 默认false添加, true不添加
	TagToLower       bool // tag的字段名字是否转换为小写, 如果本身有大写字母的话, 默认false不转
	JsonTagToHump    bool // json tag是否转为驼峰，默认为false，不转换
	UcFirstOnly      bool // 字段首字母大写的同时, 是否要把其他字母转换为小写,默认false不转换
	SeperatFile      bool // 每个struct放入单独的文件,默认false,放入同一个文件
}

func NewTable2Struct() *Table2Struct {
	return &Table2Struct{}
}

func (t *Table2Struct) Dsn(d string) *Table2Struct {
	t.dsn = d
	return t
}

func (t *Table2Struct) TagKey(r string) *Table2Struct {
	t.tagKey = r
	return t
}

func (t *Table2Struct) PackageName(r string) *Table2Struct {
	t.packageName = r
	return t
}

func (t *Table2Struct) RealNameMethod(r string) *Table2Struct {
	t.realNameMethod = r
	return t
}

func (t *Table2Struct) SavePath(p string) *Table2Struct {
	t.savePath = p
	return t
}

func (t *Table2Struct) DB(d *sql.DB) *Table2Struct {
	t.db = d
	return t
}

func (t *Table2Struct) Table(tab string) *Table2Struct {
	t.table = tab
	return t
}

func (t *Table2Struct) Prefix(p string) *Table2Struct {
	t.prefix = p
	return t
}

func (t *Table2Struct) EnableJsonTag(p bool) *Table2Struct {
	t.enableJsonTag = p
	return t
}

func (t *Table2Struct) DateToTime(d bool) *Table2Struct {
	t.dateToTime = d
	return t
}

func (t *Table2Struct) Config(c *T2tConfig) *Table2Struct {
	t.config = c
	return t
}

func (t *Table2Struct) Run() error {
	if t.config == nil {
		t.config = new(T2tConfig)
	}
	// 链接mysql, 获取db对象
	t.dialMysql()
	if t.err != nil {
		return t.err
	}

	// 获取表和字段的shcema
	tableColumns, err := t.getColumns()
	if err != nil {
		return err
	}

	// 包名
	var packageName string
	if t.packageName == "" {
		packageName = "package model\n\n"
	} else {
		packageName = fmt.Sprintf("package %s\n\n", t.packageName)
	}

	// 组装struct
	structContent := t.structContent(tableColumns)

	// 如果有引入 time.Time, 则需要引入 time 包
	var importContent string
	if strings.Contains(structContent, "time.Time") {
		importContent = "import \"time\"\n\n"
	}

	// 写入文件struct
	var savePath = t.savePath
	// 是否指定保存路径
	if savePath == "" {
		savePath = "model.go"
	}
	filePath := fmt.Sprintf("%s", savePath)
	f, err := os.Create(filePath)
	if err != nil {
		log.Println("Can not write file")
		return err
	}
	defer f.Close()

	repoContent := t.repoContent(tableColumns)

	f.WriteString(packageName + importContent + structContent + repoContent)

	cmd := exec.Command("gofmt", "-w", filePath)
	cmd.Run()

	log.Println("gen model finish!!!")

	return nil
}

func (t *Table2Struct) repoContent(tableColumns map[string][]column) string {
	// 组装struct
	var repoContent string
	for tableRealName, item := range tableColumns {
		// 去除前缀
		if t.prefix != "" {
			tableRealName = tableRealName[len(t.prefix):]
		}
		tableName := tableRealName
		structName := tableName
		if t.config.StructNameToHump {
			structName = t.camelCase(structName)
		}

		switch len(tableName) {
		case 0:
		case 1:
			tableName = strings.ToUpper(tableName[0:1])
		default:
			// 字符长度大于1时
			tableName = strings.ToUpper(tableName[0:1]) + tableName[1:]
		}
		depth := 1

		// struct
		repoName := structName + "MySQLRepository"
		repoContent += "type " + repoName + " struct {\n"
		repoContent += tab(depth) + "*common.BaseMySQLRepository\n"
		repoContent += "}\n\n"

		// new
		repoContent += "func New" + repoName + "() *" + repoName + " {"
		repoContent += tab(depth) + "return &" + repoName + "{"
		repoContent += tab(depth+1) + "common.NewBaseMySQLRepository(),"
		repoContent += tab(depth) + "}\n"
		repoContent += "}\n\n"

		// function
		queryOneFunction := "\nfunc (repo *%s) Get%s(ctx context.Context, id int64) (po *%sPO, err error) {\n\tpo = new(%sPO)\n\tquery := \"SELECT * FROM `%s%s` WHERE `id` = ?\"\n\terr = repo.Slave(ctx).GetContext(ctx, po, query, id)\n\n\tif err == sql.ErrNoRows {\n\t\treturn nil, nil\n\t}\n\n\tif err != nil {\n\t\treturn nil, errors.WithStack(err)\n\t}\n\treturn\n}\n"
		repoContent += fmt.Sprintf(queryOneFunction, repoName, structName, structName, structName, t.prefix, t.table)

		queryListFunction := "\nfunc (repo *%s) Get%ss(ctx context.Context, id int64) (pos []*%sPO, err error) {\n\tpos = []*%sPO{}\n\tquery := \"SELECT * FROM `%s%s` WHERE id = ?\"\n\terr = repo.Slave(ctx).SelectContext(ctx, &pos, query, id)\n\tif err == sql.ErrNoRows {\n\t\treturn pos, nil\n\t}\n\tif err != nil {\n\t\treturn nil, errors.WithStack(err)\n\t}\n\n\treturn\n}\n"
		repoContent += fmt.Sprintf(queryListFunction, repoName, structName, structName, structName, t.prefix, t.table)

		queryInsert := ""
		j := 0
		for i := 0; i < len(item); i++ {
			v := item[i]
			if v.ColumnOriginal == "id" {
				continue
			}

			douhao := " , "
			if j == 0 {
				douhao = " "
			}

			queryInsert += fmt.Sprintf("%s`%s` = :%s", douhao, v.ColumnOriginal, v.ColumnOriginal)
			j++
		}

		if len(queryInsert) == 0 {
			continue
		}

		queryInsertFunction := "\nfunc (repo *%s) Create%s(ctx context.Context, po *%sPO) error {\n\tquery := \"INSERT INTO `%s%s` SET%s\"\n\t\n\tret, err := repo.Master().NamedExecContext(ctx, query, po)\n\tif err != nil {\n\t\treturn errors.WithStack(err)\n\t}\n\tid, err := ret.LastInsertId()\n\tif err != nil {\n\t\treturn errors.WithStack(err)\n\t}\n\tpo.Id = id\n\n\treturn nil\n}"
		repoContent += fmt.Sprintf(queryInsertFunction, repoName, structName, structName, t.prefix, t.table, queryInsert)
		repoContent += "\n"

		updateFunction := "\nfunc (repo *%s) Update%s(ctx context.Context, po *%sPO) (updateSuccess bool, err error) {\n\tquery := \"UPDATE `%s%s` SET%s WHERE id = :id\"\n\tret, err := repo.Master().NamedExecContext(ctx, query, po)\n\tif err != nil {\n\t\treturn false, errors.WithStack(err)\n\t}\n\n\taffectedCount, err := ret.RowsAffected()\n\tif err != nil {\n\t\treturn false, errors.Wrap(err, \" get RowsAffected failed\")\n\t}\n\n\tif affectedCount == 1 {\n\t\treturn true, nil\n\t}\n\n\tif affectedCount > 1 {\n\t\treturn true, nil\n\t}\n\n\treturn false, nil\n}"
		repoContent += fmt.Sprintf(updateFunction, repoName, structName, structName, t.prefix, t.table, queryInsert)
		repoContent += "\n"
	}

	return repoContent
}

func (t *Table2Struct) structContent(tableColumns map[string][]column) string {
	structContent := ""
	for tableRealName, item := range tableColumns {
		// 去除前缀
		if t.prefix != "" {
			tableRealName = tableRealName[len(t.prefix):]
		}
		tableName := tableRealName
		structName := tableName
		if t.config.StructNameToHump {
			structName = t.camelCase(structName)
		}

		switch len(tableName) {
		case 0:
		case 1:
			tableName = strings.ToUpper(tableName[0:1])
		default:
			// 字符长度大于1时
			tableName = strings.ToUpper(tableName[0:1]) + tableName[1:]
		}
		depth := 1
		columnPOContent := ""
		columnContent := ""
		poToEntity := ""
		entityToPO := ""
		for _, v := range item {
			//structContent += tab(depth) + v.ColumnName + " " + v.Type + " " + v.Json + "\n"
			// 字段注释
			var clumnComment string
			if v.ColumnComment != "" {
				clumnComment = fmt.Sprintf(" // %s", v.ColumnComment)
			}
			columnPOContent += fmt.Sprintf("%s%s %s %s%s\n",
				tab(depth), v.ColumnName, v.Type, v.Tag, clumnComment)
			columnContent += fmt.Sprintf("%s%s %s\n", tab(depth), v.ColumnName, v.Type)
			poToEntity += fmt.Sprintf("%s%s: po.%s,\n", tab(depth+1), v.ColumnName, v.ColumnName)
			entityToPO += fmt.Sprintf("%s%s: entity.%s,\n", tab(depth+1), v.ColumnName, v.ColumnName)
		}

		structPOName := structName + "PO"
		structContent += "type " + structPOName + " struct {\n"
		structContent += columnPOContent
		structContent += tab(depth-1) + "}\n\n"

		structContent += "type " + structName + " struct {\n"
		structContent += columnContent
		structContent += tab(depth-1) + "}\n\n"

		poToentityFunction := "func To%s(po *%s) *%s {\n\tif po == nil {\n\t\treturn nil\n\t}\n\n\treturn &%s{\n%s\n}\n}\n"
		structContent += fmt.Sprintf(poToentityFunction, structName, structPOName, structName, structName, poToEntity)
		structContent += "\n"

		entityToPOFunction := "\nfunc (entity *%s) To%s() *%s {\n\treturn &%s{\n%s\n}\n}\n"
		structContent += fmt.Sprintf(entityToPOFunction, structName, structPOName, structPOName, structPOName, entityToPO)

		structContent += "/**\n"

		typeFunction := "type %s %sPO\n\nfunc (entity *%s) To%sPO() *%sPO {\n\treturn (*%sPO)(entity)\n}\n\nfunc %sPO(po *%sPO) *%s {\n\treturn (*%s)(po)\n}"
		structContent += fmt.Sprintf(typeFunction, structName, structName, structName, structName, structName, structName, structName, structName, structName, structName)
		structContent += "*/\n"

		// 添加 method 获取真实表名
		if t.realNameMethod != "" {
			structContent += fmt.Sprintf("func (%s) %s() string {\n",
				structName, t.realNameMethod)
			structContent += fmt.Sprintf("%sreturn \"%s\"\n",
				tab(depth), tableRealName)
			structContent += "}\n\n"
		}
	}

	return structContent
}

func (t *Table2Struct) dialMysql() {
	if t.db == nil {
		if t.dsn == "" {
			t.err = errors.New("dsn数据库配置缺失")
			return
		}
		t.db, t.err = sql.Open("mysql", t.dsn)
	}
	return
}

type column struct {
	ColumnName     string
	Type           string
	Nullable       string
	TableName      string
	ColumnComment  string
	Tag            string
	ColumnOriginal string
}

// Function for fetching schema definition of passed table
func (t *Table2Struct) getColumns(table ...string) (tableColumns map[string][]column, err error) {
	// 根据设置,判断是否要把 date 相关字段替换为 string
	if t.dateToTime == false {
		typeForMysqlToGo["date"] = "string"
		typeForMysqlToGo["datetime"] = "string"
		typeForMysqlToGo["timestamp"] = "string"
		typeForMysqlToGo["time"] = "string"
	}
	tableColumns = make(map[string][]column)
	// sql
	var sqlStr = `SELECT COLUMN_NAME,DATA_TYPE,IS_NULLABLE,TABLE_NAME,COLUMN_COMMENT
		FROM information_schema.COLUMNS 
		WHERE table_schema = DATABASE()`
	// 是否指定了具体的table
	if t.table != "" {
		sqlStr += fmt.Sprintf(" AND TABLE_NAME = '%s'", t.prefix+t.table)
	}
	// sql排序
	sqlStr += " order by TABLE_NAME asc, ORDINAL_POSITION asc"

	rows, err := t.db.Query(sqlStr)
	if err != nil {
		log.Println("Error reading table information: ", err.Error())
		return
	}

	defer rows.Close()

	for rows.Next() {
		col := column{}
		err = rows.Scan(&col.ColumnOriginal, &col.Type, &col.Nullable, &col.TableName, &col.ColumnComment)

		if err != nil {
			log.Println(err.Error())
			return
		}

		//col.Json = strings.ToLower(col.ColumnName)
		col.ColumnName = col.ColumnOriginal
		col.Tag = col.ColumnName
		col.ColumnComment = col.ColumnComment
		col.ColumnName = t.camelCase(col.ColumnName)
		col.Type = typeForMysqlToGo[col.Type]
		jsonTag := col.Tag
		// 字段首字母本身大写, 是否需要删除tag
		if t.config.RmTagIfUcFirsted &&
			col.ColumnName[0:1] == strings.ToUpper(col.ColumnName[0:1]) {
			col.Tag = "-"
		} else {
			// 是否需要将tag转换成小写
			if t.config.TagToLower {
				col.Tag = strings.ToLower(col.Tag)
				jsonTag = col.Tag
			}

			if t.config.JsonTagToHump {
				jsonTag = t.camelCase(jsonTag)
			}

			//if col.Nullable == "YES" {
			//	col.Json = fmt.Sprintf("`json:\"%s,omitempty\"`", col.Json)
			//} else {
			//}
		}
		if t.tagKey == "" {
			t.tagKey = "orm"
		}
		if t.enableJsonTag {
			//col.Json = fmt.Sprintf("`json:\"%s\" %s:\"%s\"`", col.Json, t.config.TagKey, col.Json)
			col.Tag = fmt.Sprintf("`%s:\"%s\" json:\"%s\"`", t.tagKey, col.Tag, jsonTag)
		} else {
			col.Tag = fmt.Sprintf("`%s:\"%s\"`", t.tagKey, col.Tag)
		}
		//columns = append(columns, col)
		if _, ok := tableColumns[col.TableName]; !ok {
			tableColumns[col.TableName] = []column{}
		}
		tableColumns[col.TableName] = append(tableColumns[col.TableName], col)
	}
	return
}

func (t *Table2Struct) camelCase(str string) string {
	// 是否有表前缀, 设置了就先去除表前缀
	if t.prefix != "" {
		str = strings.Replace(str, t.prefix, "", 1)
	}
	var text string
	//for _, p := range strings.Split(name, "_") {
	for _, p := range strings.Split(str, "_") {
		// 字段首字母大写的同时, 是否要把其他字母转换为小写
		switch len(p) {
		case 0:
		case 1:
			text += strings.ToUpper(p[0:1])
		default:
			// 字符长度大于1时
			if t.config.UcFirstOnly == true {
				text += strings.ToUpper(p[0:1]) + strings.ToLower(p[1:])
			} else {
				text += strings.ToUpper(p[0:1]) + p[1:]
			}
		}
	}
	return text
}
func tab(depth int) string {
	return strings.Repeat("\t", depth)
}
