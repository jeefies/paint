from subprocess import call

def runner():
    call(".\\main start", shell = True)

# from threading import Timer
# runner()
# Timer(60 * 60, runner, ()).start()

import os
os.rename("config.txt", "config-cdqz.txt")
os.rename("config-world.txt", "config.txt")
call(".\\main start", shell = True)