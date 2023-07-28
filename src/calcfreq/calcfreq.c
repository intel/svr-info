/* 
 * Copyright (C) 2023 Intel Corporation
 * SPDX-License-Identifier: MIT 
*/
#define _GNU_SOURCE

#include <sys/timeb.h>
#include <pthread.h>
#include <unistd.h>
#include <string.h>
#include <stdlib.h>
#include <stdio.h>
#include <sched.h>
#include <errno.h>
#include <sys/types.h>
#include <sys/stat.h>
#include <fcntl.h>
#include <sys/time.h>
#include <assert.h>

typedef unsigned long long int UINT64;
typedef long long int __int64;

#define IA32_APERF_MSR	0xe8

struct _p {

    __int64 total_time;
    __int64 total_aperf_cycles;
    __int64 iterations;
    int	id;
    int id2;
    __int64 junk[5];
} param[128];

pthread_t td[1024];

__int64 iterations = 100LL*1000000LL; // 100 million iteration as default
UINT64 len=0;
UINT64 num_cpus=0;
UINT64 start_cpu=1;
UINT64 freq;
double NsecClk;
__int64 cycles_expected, actual_cycles, running_freq, actual_aperf_cycles;
int cpu_assignment=0;
int use_aperf=0;

int BindToCpu(int cpu_num);
int check_whether_ia32_aperf_is_accessible();
UINT64 get_msr_value(int cpu, unsigned long msrNum);
UINT64 read_msr(char * msrDevPName, unsigned long msrNum, UINT64 *msrValueP);
void NopLoop(__int64 iter);
void Calibrate(UINT64   *ClksPerSec);

static inline unsigned long rdtsc ()
{
    unsigned long var;
    unsigned int hi, lo;
    asm volatile ("lfence");
    asm volatile ("rdtsc" : "=a" (lo), "=d" (hi));
    var = ((unsigned long long int) hi << 32) | lo;

    return var;
}

void BusyLoop()
{
    __int64 start, end;

    // run for about 200 milliseconds assuming a speed of 2GHz - need not be precise
    // this is done so the core has enough time to ramp up the frequency

    start = rdtsc();
    while (1) {
        end = rdtsc();
        if ((end - start) > 400000000LL) {
            break;
        }
    }

}

void execNopLoop(void* p)
{
    char *buf;
    int id, blk_start,i,j;
    __int64 start, end, delta, start_aperf, end_aperf;
    struct _p *pp;

    pp = (struct _p *)p; // cpu#
    BindToCpu(pp->id); // pin to that cpu
    pp->total_time = 0;

    // crank up the frequency to make sure it reaches the max limit
    BusyLoop();

    if (use_aperf) {
        // just do one loop
        start = rdtsc();
        start_aperf = get_msr_value(pp->id, IA32_APERF_MSR);
        NopLoop((__int64)iterations);
        end_aperf = get_msr_value(pp->id, IA32_APERF_MSR);
        end = rdtsc();
        pp->total_time = end - start;
        pp->total_aperf_cycles = end_aperf - start_aperf;

    }
    else {
        // repeat the measurement for 3 times and take the best value
        for (i=0; i < 3; i++) {
            start = rdtsc();
            NopLoop((__int64)iterations);
            end = rdtsc();
            delta = end - start;
            if (delta > pp->total_time) pp->total_time = delta;
        }
    }

}

int get_retire_per_cycle(int family, int model, int stepping) {
    /* only Intel */
    if (family == 6 /*Intel*/) {
        /* Note: this approach doesn't work for SPR, 5 is too low, six is too high, so using APERF. */
        if (model == 106 /*ICX*/ || model == 108 /*ICX*/) {
            return 5;
        }
        if (model == 63 /*HSX*/ || model == 79 /*BDX*/ || model == 86 /*BDX2*/ || model == 85 /*SKX,CLX,CPX*/) {
            return 4;
        }
    }
    return -1;
}

void get_arch(int *family, int *model, int *stepping) {
    FILE *fp = fopen("/proc/cpuinfo", "r");
    assert(fp != NULL);
    size_t n = 0;
    char *line = NULL;
    int info_count=0;
    while (getline(&line, &n, fp) > 0) {
        if (strstr(line, "model\t")) {
            sscanf(line,"model           : %d",model);
            info_count++;
        }
        if (strstr(line, "cpu family\t")) {
            sscanf(line,"cpu family           : %d",family);
            info_count++;
        }
        if (strstr(line, "stepping\t")) {
            sscanf(line,"stepping           : %d",stepping);
            info_count++;
        }
        if(info_count==3) {
            //printf("model=%d, family=%d, stepping=%d\n",*model,*family,*stepping);
            break;
        }
    }
    free(line);
    fclose(fp);
}

void Version()
{
    fprintf(stderr, "calcfreq %s\n", VERSION);
}

void Usage(const char* error)
{
    if (error) {
        fprintf(stderr, "%s\n\n", error);
    }

    fprintf(stderr, "   -t : number of physical cores to scale up to. Default=0 (only give P1 freq)\n");
    fprintf(stderr, "   -c : core count at which to start. Default=1\n");
    fprintf(stderr, "   -x : iterations in millions. Default=100000000\n");
    fprintf(stderr, "   -a : set to 1 if HT threads get consecutive cpu #s. Default=0 (alternating cpu #s)\n");
    fprintf(stderr, "   -h : display this usage information\n");
    fprintf(stderr, "   -v : display calcfreq version\n");
    fprintf(stderr, "\nExamples:\n");
    fprintf(stderr, "   ./calcfreq                  # only collect P1 Freq\n");
    fprintf(stderr, "   ./calcfreq -t4 -c2 -x10 -a1 # measure freq. with 2 to 4 cores busy at 10 iter.\n");

    if (error) {
        exit(1);
    }
    exit(0);

}

int main(int argc, char **argv)
{
    for (int i = 1; (i < argc && argv[i][0] == '-'); i++) {
        switch (argv[i][1]) {
        case 'h': {
            /* Help - print usage and exit */
            Usage((char*) 0);
        }

        case 'v': {
            Version();
            exit(0);
        }

        case 't': {
            num_cpus = atoi(&argv[i][2]);
            break;
        }

        case 'a': {
            cpu_assignment = atoi(&argv[i][2]);
            break;
        }

        case 'x': {
            iterations = (UINT64)(atoi(&argv[i][2]))*1000000LL;
            break;
        }

        case 'c': {
            start_cpu = (atoi(&argv[i][2]));
            if (start_cpu < 1) {
                start_cpu = 1;
            }
            break;
        }

        default: {
            fprintf(stderr, "Invalid Argument:%s\n", &argv[i][0]);
            Usage((char*) 0);
        }
        }
    }

    // Detect architecture to determine cycles_expected
    int family, model, stepping;
    get_arch(&family, &model, &stepping);
    if (model == 143 /*SPR*/ || model == 207 /*EMR*/) {
        use_aperf = check_whether_ia32_aperf_is_accessible();
        if (!use_aperf) {
            fprintf(stderr, "Failed to read APERF MSR.\n");
            return 1;
        }
    }
    int retiring = get_retire_per_cycle(family, model, stepping);
    if (retiring == -1 && !use_aperf) {
        fprintf(stderr, "Unsupported architecture: Family %d, Model %d, Stepping %d\n", family, model, stepping);
        return 1;
    }
    // we are executing 200 instructions and in each cycle we can retire 4 or 5 based on architecture
    cycles_expected = iterations * 200 / retiring;

    // ramp up the processor frequency and measure the TSC frequency
    BusyLoop();
    Calibrate(&freq); // Get the P1 freq
    printf("P1 freq = %lld MHz\n",freq/1000000);

    // measure specified cpu counts
    for (int idx = start_cpu; idx <= num_cpus; idx++) {
        __int64 tt, tt_aperf;
        for (int i=0, j=0; i < idx; i++, j+=2) {
            if (cpu_assignment == 1) {
                // CPU#s are assigned consecutively. i.e cpu0&1 will map to the same physical core
                param[i].id = j;
            }
            else {
                param[i].id = i;
            }
            pthread_create(&td[i], NULL, (void*)execNopLoop, (void*)&param[i]);
        }
        tt=0;
        tt_aperf=0;
        for (int i=0; i < idx; i++) {
            pthread_join(td[i], NULL);
            tt += param[i].total_time;
            tt_aperf += param[i].total_aperf_cycles;
        }
        actual_cycles = tt / idx;
        if (use_aperf) {
            actual_aperf_cycles = tt_aperf / idx;
            running_freq = (double)actual_aperf_cycles/((double)(actual_cycles)*NsecClk/(double)1000000000LL),
            printf("%d-core turbo\t%lld MHz\n", idx, running_freq/1000000);
        }
        else {
            running_freq = (__int64) ((double) cycles_expected * (double) freq / (double) actual_cycles);
            printf("%d-core turbo\t%lld MHz\n", idx, running_freq/1000000);
        }

    }
    return 0;
}

// pin to a specific cpu
int BindToCpu(int cpu_num)
{
    long status;
    cpu_set_t cs;

    CPU_ZERO (&cs);
    CPU_SET (cpu_num, &cs);
    status = sched_setaffinity (0, sizeof(cs), &cs);
    if (status < 0) {
        printf ("Error: unable to bind thread to core %d\n", cpu_num);
        exit(1);
    }
    return 1;
}

// 200 instuctions are executed per iteration and in each cycle we can retire 4 of these instructions
void NopLoop(__int64 iter)
{
    asm (
        "xor %%r9, %%r9\n\t"
        "mov %0,%%r8\n\t"
        "loop1:\n\t"
        "xor %%rax, %%rax\n\t"
        "xor %%rax, %%rax\n\t"
        "xor %%rax, %%rax\n\t"
        "xor %%rax, %%rax\n\t"
        "xor %%rax, %%rax\n\t"
        "xor %%rax, %%rax\n\t"
        "xor %%rax, %%rax\n\t"
        "xor %%rax, %%rax\n\t"
        "xor %%rax, %%rax\n\t"
        "xor %%rax, %%rax\n\t"
        "xor %%rax, %%rax\n\t"
        "xor %%rax, %%rax\n\t"
        "xor %%rax, %%rax\n\t"
        "xor %%rax, %%rax\n\t"
        "xor %%rax, %%rax\n\t"
        "xor %%rax, %%rax\n\t"
        "xor %%rax, %%rax\n\t"
        "xor %%rax, %%rax\n\t"
        "xor %%rax, %%rax\n\t"
        "xor %%rax, %%rax\n\t"
        "xor %%rax, %%rax\n\t"
        "xor %%rax, %%rax\n\t"
        "xor %%rax, %%rax\n\t"
        "xor %%rax, %%rax\n\t"
        "xor %%rax, %%rax\n\t"
        "xor %%rax, %%rax\n\t"
        "xor %%rax, %%rax\n\t"
        "xor %%rax, %%rax\n\t"
        "xor %%rax, %%rax\n\t"
        "xor %%rax, %%rax\n\t"
        "xor %%rax, %%rax\n\t"
        "xor %%rax, %%rax\n\t"
        "xor %%rax, %%rax\n\t"
        "xor %%rax, %%rax\n\t"
        "xor %%rax, %%rax\n\t"
        "xor %%rax, %%rax\n\t"
        "xor %%rax, %%rax\n\t"
        "xor %%rax, %%rax\n\t"
        "xor %%rax, %%rax\n\t"
        "xor %%rax, %%rax\n\t"
        "xor %%rax, %%rax\n\t"
        "xor %%rax, %%rax\n\t"
        "xor %%rax, %%rax\n\t"
        "xor %%rax, %%rax\n\t"
        "xor %%rax, %%rax\n\t"
        "xor %%rax, %%rax\n\t"
        "xor %%rax, %%rax\n\t"
        "xor %%rax, %%rax\n\t"
        "xor %%rax, %%rax\n\t"
        "xor %%rax, %%rax\n\t"
        "xor %%rax, %%rax\n\t"
        "xor %%rax, %%rax\n\t"
        "xor %%rax, %%rax\n\t"
        "xor %%rax, %%rax\n\t"
        "xor %%rax, %%rax\n\t"
        "xor %%rax, %%rax\n\t"
        "xor %%rax, %%rax\n\t"
        "xor %%rax, %%rax\n\t"
        "xor %%rax, %%rax\n\t"
        "xor %%rax, %%rax\n\t"
        "xor %%rax, %%rax\n\t"
        "xor %%rax, %%rax\n\t"
        "xor %%rax, %%rax\n\t"
        "xor %%rax, %%rax\n\t"
        "xor %%rax, %%rax\n\t"
        "xor %%rax, %%rax\n\t"
        "xor %%rax, %%rax\n\t"
        "xor %%rax, %%rax\n\t"
        "xor %%rax, %%rax\n\t"
        "xor %%rax, %%rax\n\t"
        "xor %%rax, %%rax\n\t"
        "xor %%rax, %%rax\n\t"
        "xor %%rax, %%rax\n\t"
        "xor %%rax, %%rax\n\t"
        "xor %%rax, %%rax\n\t"
        "xor %%rax, %%rax\n\t"
        "xor %%rax, %%rax\n\t"
        "xor %%rax, %%rax\n\t"
        "xor %%rax, %%rax\n\t"
        "xor %%rax, %%rax\n\t"
        "xor %%rax, %%rax\n\t"
        "xor %%rax, %%rax\n\t"
        "xor %%rax, %%rax\n\t"
        "xor %%rax, %%rax\n\t"
        "xor %%rax, %%rax\n\t"
        "xor %%rax, %%rax\n\t"
        "xor %%rax, %%rax\n\t"
        "xor %%rax, %%rax\n\t"
        "xor %%rax, %%rax\n\t"
        "xor %%rax, %%rax\n\t"
        "xor %%rax, %%rax\n\t"
        "xor %%rax, %%rax\n\t"
        "xor %%rax, %%rax\n\t"
        "xor %%rax, %%rax\n\t"
        "xor %%rax, %%rax\n\t"
        "xor %%rax, %%rax\n\t"
        "xor %%rax, %%rax\n\t"
        "xor %%rax, %%rax\n\t"
        "xor %%rax, %%rax\n\t"
        "xor %%rax, %%rax\n\t"
        "xor %%rax, %%rax\n\t"
        "xor %%rax, %%rax\n\t"
        "xor %%rax, %%rax\n\t"
        "xor %%rax, %%rax\n\t"
        "xor %%rax, %%rax\n\t"
        "xor %%rax, %%rax\n\t"
        "xor %%rax, %%rax\n\t"
        "xor %%rax, %%rax\n\t"
        "xor %%rax, %%rax\n\t"
        "xor %%rax, %%rax\n\t"
        "xor %%rax, %%rax\n\t"
        "xor %%rax, %%rax\n\t"
        "xor %%rax, %%rax\n\t"
        "xor %%rax, %%rax\n\t"
        "xor %%rax, %%rax\n\t"
        "xor %%rax, %%rax\n\t"
        "xor %%rax, %%rax\n\t"
        "xor %%rax, %%rax\n\t"
        "xor %%rax, %%rax\n\t"
        "xor %%rax, %%rax\n\t"
        "xor %%rax, %%rax\n\t"
        "xor %%rax, %%rax\n\t"
        "xor %%rax, %%rax\n\t"
        "xor %%rax, %%rax\n\t"
        "xor %%rax, %%rax\n\t"
        "xor %%rax, %%rax\n\t"
        "xor %%rax, %%rax\n\t"
        "xor %%rax, %%rax\n\t"
        "xor %%rax, %%rax\n\t"
        "xor %%rax, %%rax\n\t"
        "xor %%rax, %%rax\n\t"
        "xor %%rax, %%rax\n\t"
        "xor %%rax, %%rax\n\t"
        "xor %%rax, %%rax\n\t"
        "xor %%rax, %%rax\n\t"
        "xor %%rax, %%rax\n\t"
        "xor %%rax, %%rax\n\t"
        "xor %%rax, %%rax\n\t"
        "xor %%rax, %%rax\n\t"
        "xor %%rax, %%rax\n\t"
        "xor %%rax, %%rax\n\t"
        "xor %%rax, %%rax\n\t"
        "xor %%rax, %%rax\n\t"
        "xor %%rax, %%rax\n\t"
        "xor %%rax, %%rax\n\t"
        "xor %%rax, %%rax\n\t"
        "xor %%rax, %%rax\n\t"
        "xor %%rax, %%rax\n\t"
        "xor %%rax, %%rax\n\t"
        "xor %%rax, %%rax\n\t"
        "xor %%rax, %%rax\n\t"
        "xor %%rax, %%rax\n\t"
        "xor %%rax, %%rax\n\t"
        "xor %%rax, %%rax\n\t"
        "xor %%rax, %%rax\n\t"
        "xor %%rax, %%rax\n\t"
        "xor %%rax, %%rax\n\t"
        "xor %%rax, %%rax\n\t"
        "xor %%rax, %%rax\n\t"
        "xor %%rax, %%rax\n\t"
        "xor %%rax, %%rax\n\t"
        "xor %%rax, %%rax\n\t"
        "xor %%rax, %%rax\n\t"
        "xor %%rax, %%rax\n\t"
        "xor %%rax, %%rax\n\t"
        "xor %%rax, %%rax\n\t"
        "xor %%rax, %%rax\n\t"
        "xor %%rax, %%rax\n\t"
        "xor %%rax, %%rax\n\t"
        "xor %%rax, %%rax\n\t"
        "xor %%rax, %%rax\n\t"
        "xor %%rax, %%rax\n\t"
        "xor %%rax, %%rax\n\t"
        "xor %%rax, %%rax\n\t"
        "xor %%rax, %%rax\n\t"
        "xor %%rax, %%rax\n\t"
        "xor %%rax, %%rax\n\t"
        "xor %%rax, %%rax\n\t"
        "xor %%rax, %%rax\n\t"
        "xor %%rax, %%rax\n\t"
        "xor %%rax, %%rax\n\t"
        "xor %%rax, %%rax\n\t"
        "xor %%rax, %%rax\n\t"
        "xor %%rax, %%rax\n\t"
        "xor %%rax, %%rax\n\t"
        "xor %%rax, %%rax\n\t"
        "xor %%rax, %%rax\n\t"
        "xor %%rax, %%rax\n\t"
        "xor %%rax, %%rax\n\t"
        "xor %%rax, %%rax\n\t"
        "xor %%rax, %%rax\n\t"
        "xor %%rax, %%rax\n\t"
        "xor %%rax, %%rax\n\t"
        "xor %%rax, %%rax\n\t"
        "xor %%rax, %%rax\n\t"
        "xor %%rax, %%rax\n\t"
        "xor %%rax, %%rax\n\t"
        "inc %%r9\n\t"
        "cmp %%r8, %%r9\n\t"
        "jb loop1\n\t"

        ::"r"(iter));
}

static inline unsigned long long int GetTickCount()
{   //Return ns counts
    struct timeval tp;
    gettimeofday(&tp,NULL);
    return tp.tv_sec*1000+tp.tv_usec/1000;
}

// Get P1 freq
void Calibrate(UINT64   *ClksPerSec)
{
    UINT64  start;
    UINT64  end;
    UINT64  diff;

    unsigned long long int  starttick, endtick;
    unsigned long long int  tickdiff;

    endtick = GetTickCount();

    while(endtick == (starttick=GetTickCount()) );

    asm("mfence");
    start = rdtsc();
    asm("mfence");
    while((endtick=GetTickCount())  < (starttick + 500));
    asm("mfence");
    end = rdtsc();
    asm("mfence");
    //      printf("start tick=%llu, end tick=%llu\n",starttick,endtick);

    diff = end - start;
    tickdiff = endtick - starttick;
    //      printf("end=%llu,start=%llu,diff=%llu\n",end,start,diff);
    *ClksPerSec = ( diff * (UINT64)1000 )/ (unsigned long long int)(tickdiff);
    NsecClk = (double)1000000000 / (double)(__int64)*ClksPerSec;
}

UINT64 read_msr(char * msrDevPName, unsigned long msrNum, UINT64 *msrValueP)
{
    int fh;
    off_t fpos;
    ssize_t countBy;

    if ((fh= open(msrDevPName,O_RDWR))<0) {
        return 0;
    }
    if ((fpos= lseek(fh,msrNum,SEEK_SET)),0) {
        return 0;
    }
    if ((countBy= read(fh,msrValueP,sizeof(UINT64)))<0) {
        close(fh);
        return 0;
    }
    else if (countBy!=sizeof(UINT64)) {
        close(fh);
        return 0;
    }
    close(fh);
    return 1;
}

int check_whether_ia32_aperf_is_accessible()
{
    char msrDevPName[1024];
    UINT64 msrValue;
    int cpu=0;

    snprintf(msrDevPName,sizeof(msrDevPName)-1,"/dev/cpu/%d/msr",cpu);
    if (read_msr(msrDevPName, IA32_APERF_MSR, &msrValue) == 0) {
        //fprintf(stderr,"\n** Unable to read IA32_APERF MSR. So, frequency will be estimated\n");
        return 0;
    }
    return 1;
}

UINT64 get_msr_value(int cpu, unsigned long msrNum)
{
    char msrDevPName[1024];
    UINT64 msrValue;

    snprintf(msrDevPName,sizeof(msrDevPName)-1,"/dev/cpu/%d/msr",cpu);
    if (read_msr(msrDevPName, msrNum, &msrValue) == 0) {
        fprintf(stderr, "failed to read msr %lx\n", msrNum);
        exit(1);
    }
    return msrValue;
}

