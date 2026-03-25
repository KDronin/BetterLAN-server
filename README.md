# BetterLAN-server

Use with the mod ["BetterLAN"](https://github.com/KDronin/BetterLAN).

First, you need a server. There are no requirements for hardware configuration; you can even use an Orange Pi. The only requirement is to have sufficient bandwidth to withstand the gameplay you plan for.

Based on my experience (which may not be accurate), if you allocate 1.5Mbps of bandwidth per player, your game will hardly encounter bandwidth bottlenecks.

# Install

For Linux systems, simply enter the following command in the terminal:

```
curl -sSL https://raw.githubusercontent.com/KDronin/BetterLAN-server/refs/heads/main/install.sh | bash
```

For Windows systems, run PowerShell as an administrator and enter the following command in it:

```
Set-ExecutionPolicy Bypass -Scope Process -Force; iex (Invoke-RestMethod -Uri 'https://raw.githubusercontent.com/KDronin/BetterLAN-server/refs/heads/main/install.ps1')
```
