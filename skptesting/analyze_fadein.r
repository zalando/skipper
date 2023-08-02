#!/usr/bin/env Rscript

library('getopt');
library('lattice');

# 0=no param, 1=required, 2=optional
spec <- matrix( c(
               'help', 'h', 0, "logical", "print help message",
               'file', 'f', 2, "character", "path to filename to read, defaults to STDIN",
               'output', 'o', 2, "character", "path to filename to write to, defaults to 'graph.png'",
               'sep', 's', 2, "character", "CSV separator, default to ','",
               'title', 't', 2, "character", "Title for the graph 'Test'"),
               byrow=T, ncol=5
          )
opt <- getopt(spec);
## --help
if ( !is.null(opt$help) ) {
  self = commandArgs()[1];
  cat(paste("Usage: ",self," [--help|-h] [--file|-f <path>] [--output|-o <path>] [--sep|-s <char>] [--title|-t <string>]\n", sep=""));
  q(status=1);
}
## input - STDIN or file
if (is.null(opt$file)) {
  print("read from STDIN")
  con <- file(description="stdin",open="r")
} else {
  con <- file(opt$file,open="r")
}
## output - STDOUT or file
if (is.null(opt$output)) {
  out <- "/dev/stdout"
} else {
  out <- opt$output
}
## sep
if (is.null(opt$sep)) {
    opt$sep <- ","
}
## title
if (is.null(opt$title)) {
  title <- paste("Test", opt$output)
} else {
  title <- opt$title
}

## set that we write to png file
png(out)

datWithT <- read.csv(con, sep=opt$sep)
dat <- datWithT[,-1]

## hack to parameterize formula
## we have to create c(a,b,c,d,e,f) where a,b,..,f depend on the read CSV we don't know yet
strFactor <- paste(
    "c(",
    paste(names(dat), collapse=","),
    ")",
    collapse=" ", sep="")
## create the formula c(a,b,c,d,e,f) ~ c(1:nrow(dat))
f <- eval(parse(text=strFactor)) ~ c(1:nrow(dat))
colors <- c("blue", "green", "red", "black", "magenta", "yellow", "cyan")

xyplot(f,
       data=dat,
       xlab="iterations", ylab="hits", ylim=c(0, max(dat)+200), between=list(x=0,y=100),
       key=list(space="bottom",
         points=list(col=head(colors, n=ncol(dat)), pch=1),
         text=list(names(dat)), cex=0.6),
       main=title, col=rep(colors, each=nrow(dat)))
