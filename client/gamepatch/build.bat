@echo off
rem Compila o GamePatch.dll (32 bits, runtime estatico: nao depende de DLL nenhum).
rem Precisa do Visual Studio 2022 com C++ — Build Tools ou Community.
setlocal
cd /d "%~dp0"
call "C:\Program Files (x86)\Microsoft Visual Studio\2022\BuildTools\VC\Auxiliary\Build\vcvars32.bat" >nul 2>&1
if not defined VCToolsInstallDir call "C:\Program Files\Microsoft Visual Studio\18\Community\VC\Auxiliary\Build\vcvars32.bat" >nul 2>&1
if not defined VCToolsInstallDir exit /b 1
if not exist out mkdir out
cl /nologo /LD /MT /O2 /W4 /EHsc /Fo:out\ gamepatch.cpp timerfields.cpp acessoriopct.cpp olhosdeaguia.cpp divisao.cpp macromago.cpp zoomcam.cpp alvos.cpp camadas.cpp pincel.cpp d3dpainel.cpp overlay.cpp loja.cpp lojarede.cpp icones.cpp servidor.cpp /link user32.lib gdi32.lib d3d9.lib ws2_32.lib /OUT:out\GamePatch.dll /NOLOGO || exit /b 1
echo.
echo GamePatch.dll gerado em %~dp0out
