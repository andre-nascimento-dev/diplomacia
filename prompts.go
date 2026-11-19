package main

import (
	"cmp"
	"context"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type prompt struct {
	name, description string
	args              []*mcp.PromptArgument
	text              func(a map[string]string) string
}

func arg(name, description string, required bool) *mcp.PromptArgument {
	return &mcp.PromptArgument{Name: name, Description: description, Required: required}
}

func optional(prefix, value string) string {
	if strings.TrimSpace(value) == "" {
		return ""
	}
	return prefix + value + "\n"
}

var prompts = []prompt{
	{
		name:        "responder",
		description: "Redige uma resposta para uma conversa, no tom dela, e envia depois da sua confirmação",
		args:        []*mcp.PromptArgument{arg("conversa", "nome da pessoa ou do grupo", true), arg("instrucao", "o que a resposta deve dizer", false)},
		text: func(a map[string]string) string {
			return "Encontre a conversa \"" + a["conversa"] + "\" com list_chats.\n" +
				"Leia as últimas mensagens com read_messages. Abra com get_media os arquivos que importam para a resposta.\n" +
				"Leia o tom da conversa com get_tone e siga esse tom.\n" +
				optional("Na resposta, diga: ", a["instrucao"]) +
				"Mostre o rascunho. Envie com send_message só depois da minha confirmação."
		},
	},
	{
		name:        "resumir",
		description: "Resume uma conversa: fatos, decisões, pedidos e prazos",
		args:        []*mcp.PromptArgument{arg("conversa", "nome da pessoa ou do grupo", true), arg("periodo", "por exemplo hoje, esta semana ou desde segunda", false)},
		text: func(a map[string]string) string {
			return "Encontre a conversa \"" + a["conversa"] + "\" com list_chats.\n" +
				"Leia as mensagens com read_messages e pagine com before até cobrir o período.\n" +
				optional("Período: ", a["periodo"]) +
				"Use as transcrições dos áudios. Abra com get_media os arquivos que mudam o sentido da conversa.\n" +
				"Resuma em lista: fatos, decisões, pedidos abertos e prazos. Diga quem disse cada item."
		},
	},
	{
		name:        "pendentes",
		description: "Lista as conversas que esperam uma resposta sua",
		args:        []*mcp.PromptArgument{arg("quantidade", "quantas conversas recentes olhar, padrão 20", false)},
		text: func(a map[string]string) string {
			return "Liste as " + cmp.Or(a["quantidade"], "20") + " conversas mais recentes com list_chats.\n" +
				"Em cada conversa, leia as últimas mensagens com read_messages.\n" +
				"Mostre só as conversas em que a última mensagem não é minha e pede uma resposta.\n" +
				"Para cada uma, diga quem é, o que a pessoa pede e desde quando espera. Comece pela mais antiga."
		},
	},
	{
		name:        "tom",
		description: "Mostra ou grava o tom de uma conversa, ou o tom padrão",
		args:        []*mcp.PromptArgument{arg("conversa", "nome da pessoa ou do grupo; vazio usa o tom padrão", false), arg("tom", "o tom novo; vazio mostra o tom atual e sugere um", false)},
		text: func(a map[string]string) string {
			target := "o tom padrão"
			if a["conversa"] != "" {
				target = "o tom da conversa \"" + a["conversa"] + "\" (encontre a conversa com list_chats)"
			}
			if a["tom"] != "" {
				return "Grave " + target + " com set_tone. Tom: " + a["tom"]
			}
			return "Mostre " + target + " com get_tone.\n" +
				"Leia as últimas mensagens que eu enviei nessa conversa e sugira um tom em uma ou duas frases.\n" +
				"Grave com set_tone só depois da minha confirmação."
		},
	},
}

func addPrompts(s *mcp.Server) {
	for _, p := range prompts {
		s.AddPrompt(&mcp.Prompt{Name: p.name, Description: p.description, Arguments: p.args},
			func(_ context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
				return &mcp.GetPromptResult{Description: p.description, Messages: []*mcp.PromptMessage{
					{Role: "user", Content: &mcp.TextContent{Text: p.text(req.Params.Arguments)}},
				}}, nil
			})
	}
}
