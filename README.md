# Importador de saves muOS ↔ Knulli

Aplicativo SDL2 para copiar saves e save states entre muOS e Knulli no mesmo cartão 2 de um Anbernic RG40XXV. O programa não move nem apaga os arquivos de origem.

## Instalação no Knulli

1. Inicialize o cartão 2 pelo Knulli e confirme que ele está selecionado como armazenamento externo.
2. Extraia o conteúdo do pacote na raiz do cartão 2. A estrutura final deve conter:

   ```text
   /system/muos-save-importer/muos-save-importer
   /system/muos-save-importer/systems.json
   /roms/ports/Importar Saves muOS.sh
   ```

3. No Knulli, atualize as listas de jogos.
4. Abra **Ports → Importar Saves muOS**.

O muOS precisa ter criado `MUOS/save/file` ou `MUOS/save/state` no mesmo cartão. Os saves do Knulli ficam sob `saves/<sistema>`.

## Controles

- D-pad: navegar.
- A: confirmar.
- B: voltar ou ignorar o arquivo atual.
- L1 na tela de conflito: aplicar a ação selecionada a todos os conflitos restantes.

Save states podem ser incompatíveis quando os firmwares usam versões ou núcleos diferentes. O aplicativo mostra esse risco antes da operação e preserva sempre a origem.

## Mapeamento de sistemas

`systems.json` contém os aliases de núcleos e sistemas. Ele pode ser editado para acrescentar plataformas. Quando um save corresponde a mais de um sistema — ou a nenhum — o aplicativo solicita o destino na tela.

O reconhecimento combina:

- pasta/nome do núcleo no caminho do save;
- aliases de `systems.json`;
- nome do save comparado aos nomes das ROMs em `/userdata/roms/<sistema>`.

## Segurança e dados persistentes

- Toda cópia é escrita primeiro como `.importing`, sincronizada e validada por SHA-256 antes da substituição.
- O original não é removido.
- O índice incremental fica em `/userdata/system/muos-save-importer/index.json`.
- Índices inválidos são renomeados para `index.json.corrupt` e reconstruídos.
- Relatórios ficam em `/userdata/system/logs/muos-save-importer/` e não incluem o conteúdo dos saves.

## Desenvolvimento

O núcleo de sincronização usa apenas a biblioteca padrão do Go. A interface Linux usa `go-sdl2` e SDL2.

```powershell
go test ./internal/...
```

Para gerar o binário aarch64, o ambiente precisa de `aarch64-linux-gnu-gcc` e bibliotecas de desenvolvimento SDL2 para arm64:

```sh
make build-arm64
```

Depois, no Windows/PowerShell:

```powershell
./scripts/package.ps1 -Binary ./build/muos-save-importer
```

O pacote será criado em `dist/muos-save-importer-knulli`.

## Limitações conhecidas

- A compatibilidade real de cada save state depende do núcleo e da versão usados nos dois firmwares.
- Emuladores standalone podem usar formatos e diretórios próprios. Eles devem ser adicionados a `systems.json` e validados no aparelho.
- O binário aarch64 deve ser testado no hardware, pois este repositório não inclui uma imagem do Knulli nem um RG40XXV emulado.

## Como gerar o executável e instalar no Knulli

### Opção recomendada: GitHub Actions

Esta opção não exige instalar compilador ARM ou SDL2 no computador.

1. Envie o projeto para um repositório no GitHub.
2. Abra a aba **Actions** do repositório.
3. Selecione o workflow **build-knulli**.
4. Clique em **Run workflow** e confirme em **Run workflow** novamente.
5. Aguarde o workflow terminar com sucesso.
6. Abra a execução concluída e, na seção **Artifacts**, baixe `muos-save-importer-knulli-arm64`.
7. Extraia o arquivo baixado. Dentro dele estará `muos-save-importer-knulli-arm64.zip`, que é o pacote para o cartão 2.

O workflow compila o executável Linux ARM64 com SDL2 e prepara automaticamente esta estrutura:

```text
muos-save-importer-knulli/
├── system/
│   └── muos-save-importer/
│       ├── muos-save-importer
│       └── systems.json
└── roms/
    └── ports/
        └── Importar Saves muOS.sh
```

### Opção local: Linux ARM64 ou compilação cruzada

Em Ubuntu/Debian ARM64, instale as dependências:

```sh
sudo apt update
sudo apt install -y golang-go gcc pkg-config libsdl2-dev zip
```

Compile o executável:

```sh
mkdir -p build
CGO_ENABLED=1 GOOS=linux GOARCH=arm64 \
  go build -trimpath -ldflags="-s -w" \
  -o build/muos-save-importer ./cmd/importer
```

Monte o pacote:

```sh
PACKAGE="dist/muos-save-importer-knulli"
mkdir -p "$PACKAGE/system/muos-save-importer" "$PACKAGE/roms/ports"
install -m 0755 build/muos-save-importer "$PACKAGE/system/muos-save-importer/muos-save-importer"
install -m 0644 assets/systems.json "$PACKAGE/system/muos-save-importer/systems.json"
install -m 0755 "packaging/Importar Saves muOS.sh" "$PACKAGE/roms/ports/Importar Saves muOS.sh"
cd dist
zip -r muos-save-importer-knulli-arm64.zip muos-save-importer-knulli
```

Não use `GOOS=linux GOARCH=arm64 CGO_ENABLED=0`: a interface depende da biblioteca nativa SDL2.

### Instalação no cartão 2

1. Desligue completamente o RG40XXV.
2. Retire o cartão 2 e conecte-o ao computador. Para acesso direto pelo Windows, o cartão deve estar em exFAT.
3. Extraia `muos-save-importer-knulli-arm64.zip` em uma pasta temporária.
4. Abra a pasta `muos-save-importer-knulli` extraída.
5. Copie as pastas `system` e `roms` para a raiz do cartão 2, aceitando a mesclagem com as pastas existentes. Não copie a pasta externa `muos-save-importer-knulli` para o cartão.
6. Recoloque o cartão 2, ligue o aparelho com Knulli e confirme em **System Settings → Storage** que o cartão externo está selecionado.
7. No menu principal do Knulli, use **Game Settings → Update Gamelists**.
8. Abra **Ports → Importar Saves muOS**.

Se o item aparecer em Ports, mas não abrir, conecte-se ao Knulli por SSH e restaure as permissões:

```sh
chmod +x "/userdata/roms/ports/Importar Saves muOS.sh"
chmod +x "/userdata/system/muos-save-importer/muos-save-importer"
```

Confira também se os arquivos estão exatamente nestes caminhos:

```text
/userdata/roms/ports/Importar Saves muOS.sh
/userdata/system/muos-save-importer/muos-save-importer
/userdata/system/muos-save-importer/systems.json
/userdata/MUOS/save/file
/userdata/MUOS/save/state
```

Os dois últimos diretórios são criados pelo muOS. É suficiente que pelo menos um deles exista. Logs de inicialização e importação podem ser consultados em `/userdata/system/logs/muos-save-importer/`.
