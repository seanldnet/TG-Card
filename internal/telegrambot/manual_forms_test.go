package telegrambot

import "testing"

func TestParseManualFormFields(t *testing.T) {
	fields,err:=parseManualFormFields(`{"fields":[{"name":"account","label":{"zh-CN":"游戏账号","en":"Account"},"type":"text","required":true,"min_length":2,"max_length":40},{"key":"region","label":"区服","type":"select","required":true,"options":["亚洲","欧洲"]}]}`)
	if err!=nil{t.Fatal(err)}
	if len(fields)!=2||fields[0].Key!="account"||fields[0].Label!="游戏账号"||!fields[0].Required||fields[0].MaxLength!=40{t.Fatalf("unexpected first field: %+v",fields)}
	if fields[1].Key!="region"||len(fields[1].Options)!=2||fields[1].Type!="select"{t.Fatalf("unexpected second field: %+v",fields[1])}
}

func TestParseManualFormFieldsRejectsUnsupportedOrUnsafeSchema(t *testing.T) {
	cases:=[]string{
		`{"fields":[]}`,
		`{"fields":[{"name":"x","type":"password"}]}`,
		`{"fields":[{"name":"x"},{"name":"x"}]}`,
		`{"fields":[{"name":"x","type":"select"}]}`,
		`{"fields":[{"name":"x","min_length":20,"max_length":2}]}`,
	}
	for _,input:=range cases{if _,err:=parseManualFormFields(input);err==nil{t.Fatalf("expected schema rejection: %s",input)}}
}
