#include "stdio.h"
#include "unistd.h"

int main()
{

  char * ptr = NULL;
  int i=0;
  int retval;

  printf("\n Starting \n");
  while(1) {
     retval = access("/tmp/killme", F_OK);

     if(retval == 0) {
         *ptr = 'A'; 
     }

     printf("\n Iteration: %d retval = %d \n\n", i++, retval);
     sleep(5);

  }

}

