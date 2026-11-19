package main

import (
	"context"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestPromptsReachTheClient(t *testing.T) {
	ctx := context.Background()
	s := mcp.NewServer(&mcp.Implementation{Name: "t"}, nil)
	addPrompts(s)
	st, ct := mcp.NewInMemoryTransports()
	if _, err := s.Connect(ctx, st, nil); err != nil {
		t.Fatal(err)
	}
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "c"}, nil).Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer cs.Close()

	list, err := cs.ListPrompts(ctx, nil)
	if err != nil || len(list.Prompts) != len(prompts) {
		t.Fatalf("ListPrompts = %v, %v", list, err)
	}
	cases := []struct {
		name string
		args map[string]string
		want []string
	}{
		{"responder", map[string]string{"conversa": "Ana", "instrucao": "chego às 9"}, []string{`"Ana"`, "get_tone", "Na resposta, diga: chego às 9", "confirmação"}},
		{"resumir", map[string]string{"conversa": "Obra"}, []string{`"Obra"`, "prazos"}},
		{"pendentes", nil, []string{"Liste as 20 conversas"}},
		{"tom", map[string]string{"tom": "Formal"}, []string{"Grave o tom padrão com set_tone. Tom: Formal"}},
		{"tom", map[string]string{"conversa": "Ana"}, []string{`"Ana"`, "get_tone", "sugira"}},
	}
	for _, c := range cases {
		res, err := cs.GetPrompt(ctx, &mcp.GetPromptParams{Name: c.name, Arguments: c.args})
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		text := res.Messages[0].Content.(*mcp.TextContent).Text
		for _, w := range c.want {
			if !strings.Contains(text, w) {
				t.Errorf("%s %v: falta %q em %q", c.name, c.args, w, text)
			}
		}
	}
}
