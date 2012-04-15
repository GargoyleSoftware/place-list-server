# Create your views here.
import os
import re
from datetime import datetime
from urllib import unquote_plus

from django.core.urlresolvers import reverse
from django.views.generic.simple import direct_to_template
from django.shortcuts import render_to_response, get_object_or_404, redirect
from django.http import HttpResponse
from django.template import RequestContext
from django.conf import settings


def redirect(request, spotify_uri):
  # render them to the settlement table template
  return render_to_response('redirect.html',
                            locals(),
                            context_instance=RequestContext(request))
