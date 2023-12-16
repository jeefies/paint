from subprocess import call

def runner():
    call(".\\main start", shell = True)

from threading import Timer
runner()
Timer(60 * 60, runner, ()).start()