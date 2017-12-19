import os
import subprocess

TAR_PATH = "/work-dir"

def main():
    base_img = 'gcr.io/google-appengine/debian9:latest'
    dst_img = 'gcr.io/dlorenc-vmtest2/nodocker:latest'
    tars = [f for f in os.listdir(TAR_PATH) if f.endswith('.tar.gz')]
    for tar in sorted(tars):
        subprocess.check_call(
            ['/app/appender.par',
             '--tarball', '%s' % os.path.join(TAR_PATH, tar),
             '--dst-image', dst_img,
             '--src-image', base_img])
        base_img = dst_img

if __name__ == "__main__":
    main()