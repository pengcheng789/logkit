package linuxaudit

import (
	"strconv"
	"strings"
	"sync"
	"unicode"

	"github.com/qiniu/log"
	"github.com/qiniu/logkit/conf"
	"github.com/qiniu/logkit/parser"
	. "github.com/qiniu/logkit/parser/config"
	. "github.com/qiniu/logkit/utils/models"
)

const DefaultCheckKey = 5

type Parser struct {
	name                 string
	disableRecordErrData bool
	numRoutine           int
	keepRawData          bool
}

func init() {
	parser.RegisterConstructor(TypeLinuxAudit, NewParser)
}

func NewParser(c conf.MapConf) (parser.Parser, error) {
	name, _ := c.GetStringOr(KeyParserName, "")
	disableRecordErrData, _ := c.GetBoolOr(KeyDisableRecordErrData, false)
	keepRawData, _ := c.GetBoolOr(KeyKeepRawData, false)
	numRoutine := MaxProcs
	if numRoutine == 0 {
		numRoutine = 1
	}
	return &Parser{
		name:                 name,
		disableRecordErrData: disableRecordErrData,
		numRoutine:           numRoutine,
		keepRawData:          keepRawData,
	}, nil
}

func (p *Parser) Parse(lines []string) ([]Data, error) {
	var (
		lineLen    = len(lines)
		datas      = make([]Data, lineLen)
		se         = &StatsError{}
		numRoutine = p.numRoutine

		sendChan   = make(chan parser.ParseInfo)
		resultChan = make(chan parser.ParseResult)
		wg         = new(sync.WaitGroup)
	)
	if lineLen < numRoutine {
		numRoutine = lineLen
	}

	for i := 0; i < numRoutine; i++ {
		wg.Add(1)
		go parser.ParseLine(sendChan, resultChan, wg, true, p.parse)
	}

	go func() {
		wg.Wait()
		close(resultChan)
	}()

	go func() {
		for idx, line := range lines {
			sendChan <- parser.ParseInfo{
				Line:  line,
				Index: idx,
			}
		}
		close(sendChan)
	}()

	var parseResultSlice = make(parser.ParseResultSlice, lineLen)
	for resultInfo := range resultChan {
		parseResultSlice[resultInfo.Index] = resultInfo
	}

	se.DatasourceSkipIndex = make([]int, lineLen)
	datasourceIndex := 0
	dataIndex := 0
	for _, parseResult := range parseResultSlice {
		if len(parseResult.Line) == 0 {
			se.DatasourceSkipIndex[datasourceIndex] = parseResult.Index
			datasourceIndex++
			continue
		}

		if parseResult.Err != nil {
			se.AddErrors()
			se.LastError = parseResult.Err.Error()
			errData := make(Data)
			if !p.disableRecordErrData {
				errData[KeyPandoraStash] = parseResult.Line
			} else if !p.keepRawData {
				se.DatasourceSkipIndex[datasourceIndex] = parseResult.Index
				datasourceIndex++
			}
			if p.keepRawData {
				errData[KeyRawData] = parseResult.Line
			}
			if !p.disableRecordErrData || p.keepRawData {
				datas[dataIndex] = errData
				dataIndex++
			}
			continue
		}
		if len(parseResult.Data) < 1 { //数据为空时不发送
			se.LastError = "parsed no data by line " + parseResult.Line
			se.AddErrors()
			continue
		}
		se.AddSuccess()
		if p.keepRawData {
			parseResult.Data[KeyRawData] = parseResult.Line
		}
		datas[dataIndex] = parseResult.Data
		dataIndex++
	}

	se.DatasourceSkipIndex = se.DatasourceSkipIndex[:datasourceIndex]
	datas = datas[:dataIndex]
	if se.Errors == 0 {
		return datas, nil
	}
	return datas, se
}

func (p *Parser) Name() string {
	return p.name
}

func (p *Parser) Type() string {
	return TypeLinuxAudit
}

func (p *Parser) parse(line string) (Data, error) {
	var (
		data   = make(Data)
		tmp    string
		key    string
		prefix bool
	)

	for idx, c := range line {
		if c == '\'' {
			prefix = !prefix
			p.processSubLine(tmp, line[idx+1:], key, data)
			tmp, key = "", ""
		}

		if prefix || c == '"' {
			continue
		}

		if c == '=' {
			key = tmp
			tmp = ""
			continue
		}

		if unicode.IsSpace(c) {
			processSpace(key, tmp, data)
			tmp, key = "", ""
			continue
		}

		tmp += string(c)
	}

	if key != "" {
		if strings.HasSuffix(tmp, ":") {
			tmp = strings.TrimSuffix(tmp, ":")
		}
		setData(key, tmp, data)
	}
	return data, nil
}

func processSpace(key, tmp string, data Data) {
	if key == "" {
		return
	}

	if strings.HasSuffix(tmp, ":") {
		tmp = strings.TrimSuffix(tmp, ":")
	}
	if key == "msg" && strings.HasPrefix(tmp, "audit(") {
		if getTimestampID(tmp, data) {
			return
		}
	}
	tmp = strings.TrimSuffix(tmp, ":")
	setData(key, tmp, data)

	return
}

func getTimestampID(tmp string, data Data) bool {
	tmp = strings.TrimPrefix(tmp, "audit(")
	tmp = strings.TrimSuffix(tmp, ")")
	arr := strings.SplitN(tmp, ":", 2)
	if len(arr) < 2 {
		return false
	}

	timestamp := strings.TrimSpace(arr[0])
	if idx := strings.Index(arr[0], "."); idx != -1 {
		timestamp = arr[0][:idx] + arr[0][idx+1:]
	}
	tm, err := GetTime(timestamp)
	if err != nil {
		log.Errorf("parse msg timestamp: %s failed: %v", timestamp, err)
		data["msg_timestamp"] = strings.TrimSpace(arr[0])
	} else {
		data["msg_timestamp"] = FormatWithUserOption("", 0, tm)
	}

	data["msg_id"] = strings.TrimSpace(arr[1])
	return true
}

func (p *Parser) processSubLine(tmp, tmpLine, key string, data Data) {
	if key == "" {
		return
	}

	if tmp != "" {
		tmp = strings.TrimSuffix(tmp, ":")
		setData(key, tmp, data)
		return
	}

	suffix := strings.Index(tmpLine, "'")
	if suffix != -1 {
		value, err := p.parse(tmpLine[:suffix+1])
		if err != nil {
			log.Errorf("parse line: %s failed: %v", tmpLine[:suffix+1], err)
		} else {
			setData(key, value, data)
		}
	}
	return
}

func setData(key string, value interface{}, data Data) {
	if key == "" {
		return
	}
	_, ok := data[key]
	if !ok {
		data[key] = value
		return
	}

	i := 1
	finalKey := key + "_" + strconv.Itoa(i)
	for ; i <= DefaultCheckKey; i++ {
		_, ok = data[finalKey]
		if !ok {
			data[finalKey] = value
			return
		}
		finalKey = key + "_" + strconv.Itoa(i)
	}
	data[key] = value
}

func (t *Parser) ServerConfig() map[string]interface{} {
	config := make(map[string]interface{})
	config[KeyType] = TypeLinuxAudit
	config[ProcessAt] = Server
	config["key"] = "arr"

	return config
}
