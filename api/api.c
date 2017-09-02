// +build ignore

#include <stdio.h>
#include <stdlib.h>
#include <errno.h>
#include <unistd.h>
#include <arpa/inet.h>
#include <sys/time.h>
#include <string.h>

int nameport = 8328;

int nameapi(const char name[64], char ipport[64]) {
    int retried = 0;
    while (1) {
        int sock = socket(AF_INET, SOCK_DGRAM, 0);
        if (sock < 0) {
            return -1;
        }
        struct timeval tv;
        tv.tv_sec = 0;
        tv.tv_usec = 20000;
        if (setsockopt(sock, SOL_SOCKET, SO_RCVTIMEO, &tv, sizeof(tv)) != 0) {
            close(sock);
            return -2;
        }
        struct sockaddr_in servaddr;
        servaddr.sin_family = AF_INET;
        servaddr.sin_port = htons(nameport);
        servaddr.sin_addr.s_addr = inet_addr("127.0.0.1");
        char buf[64] = {'\0'};
        gettimeofday(&tv, NULL);
        long long seq = 1000000 * tv.tv_sec + tv.tv_usec;
        int n = snprintf(buf, sizeof(buf), "%lld,%s", seq, name);
        sendto(sock, buf, n, 0, (struct sockaddr *)&servaddr, sizeof(servaddr));
        int ret = recv(sock, buf, sizeof(buf), 0);
        if (ret <= 0) {
            close(sock);
            return -3;
        }
        long long _seq = 0;
        sscanf(buf, "%lld,%s", &_seq, ipport);
        if (seq != _seq) {
            close(sock);
            if (!retried) {
                retried = !retried;
                continue;
            }
            return -4;
        }
        close(sock);
        if (strlen(ipport) == 0) {
            if (!retried) {
                retried = !retried;
                continue;
            }
            return -5;
        }
        break;
    }
    return 0;
}

int main()
{
    const char name[64] = "namesrv.ns";
    char ipport[64] = {'\0'};
    int ret = nameapi(name, ipport);
    printf("ret: %d, ipport: %s\n", ret, ipport);
    return 0;
}
