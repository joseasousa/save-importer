param(
    [Parameter(Mandatory = $true)]
    [string]$Binary,
    [string]$Output = "dist/muos-save-importer-knulli"
)

$ErrorActionPreference = "Stop"
$resolvedBinary = (Resolve-Path -LiteralPath $Binary).Path
$resolvedOutput = [System.IO.Path]::GetFullPath((Join-Path $PWD $Output))
New-Item -ItemType Directory -Force -Path "$resolvedOutput/system/muos-save-importer", "$resolvedOutput/roms/ports" | Out-Null
Copy-Item -LiteralPath $resolvedBinary -Destination "$resolvedOutput/system/muos-save-importer/muos-save-importer"
Copy-Item -LiteralPath "assets/systems.json" -Destination "$resolvedOutput/system/muos-save-importer/systems.json"
Copy-Item -LiteralPath "packaging/Importar Saves muOS.sh" -Destination "$resolvedOutput/roms/ports/Importar Saves muOS.sh"
Copy-Item -LiteralPath "README.md" -Destination "$resolvedOutput/LEIA-ME.md"
Write-Output "Pacote criado em $resolvedOutput"
