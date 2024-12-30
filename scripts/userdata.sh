#!/bin/bash

# *** INSERT SERVER DOWNLOAD URL BELOW ***
# Do not add any spaces between your link and the "=", otherwise it won't work. EG: MINECRAFTSERVERURL=https://urlexample

MINECRAFTSERVERURL="https://piston-data.mojang.com/v1/objects/4707d00eb834b446575d89a61a11b5d548d8c001/server.jar"

# Download Java
yum install -y java-21-amazon-corretto-headless

# Install MC Java server in a directory we create
adduser minecraft
mkdir -p /opt/minecraft/server/
cd /opt/minecraft/server

# Download server jar file from Minecraft official website
curl -O $MINECRAFTSERVERURL

# Generate Minecraft server files and create script
chown -R minecraft:minecraft /opt/minecraft/
java -Xmx1300M -Xms1300M -jar server.jar nogui
sleep 40
sed -i 's/false/true/p' eula.txt
touch start
printf '#!/bin/bash\njava -Xmx1300M -Xms1300M -jar server.jar nogui\n' >> start
chmod +x start
sleep 1
touch stop
printf '#!/bin/bash\nkill -9 $(ps -ef | pgrep -f "java")' >> stop
chmod +x stop
sleep 1

# Create SystemD Script to run Minecraft server jar on reboot
cd /etc/systemd/system/
touch minecraft.service
printf '[Unit]\nDescription=Minecraft Server on start up\nWants=network-online.target\n[Service]\nUser=minecraft\nWorkingDirectory=/opt/minecraft/server\nExecStart=/opt/minecraft/server/start\nStandardInput=null\n[Install]\nWantedBy=multi-user.target' >> minecraft.service
sudo systemctl daemon-reload
sudo systemctl enable minecraft.service
sudo systemctl start minecraft.service

# Copy ec2-user's authorized keys to the minecraft user
sudo -u minecraft mkdir -p /home/minecraft/.ssh
sudo cp /home/ec2-user/.ssh/authorized_keys /home/minecraft/.ssh/
sudo chown -R minecraft:minecraft /home/minecraft/.ssh
sudo chmod 700 /home/minecraft/.ssh
sudo chmod 600 /home/minecraft/.ssh/authorized_keys

# Allow the minecraft user to use sudo without a password
echo "minecraft ALL=(ALL) NOPASSWD:ALL" | sudo tee /etc/sudoers.d/minecraft
sudo chmod 440 /etc/sudoers.d/minecraft

# End script
